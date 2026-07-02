package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// State is the full, freshly-sorted application graph returned by every mutating
// method so the frontend can replace its state in one go. Bundling subjects and
// tasks together keeps them from drifting out of sync (e.g. deleting a subject
// also ungroups its tasks). The focus log is served separately.
type State struct {
	Subjects []*Subject `json:"subjects"`
	Tasks    []*Task    `json:"tasks"`
	Settings Settings   `json:"settings"`
}

// App is the Wails-bound application. Every mutating method returns the full,
// freshly-sorted State so the frontend can replace its state in one go.
type App struct {
	ctx     context.Context
	store   *Store
	initErr error
	// now returns the current time; injectable so date-dependent scheduling is
	// deterministic in tests. Defaults to time.Now.
	now func() time.Time
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{now: time.Now}
}

// startup is called when the app starts. The context is saved so we can call
// the runtime methods, and the on-disk store is loaded.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	store, err := NewStore()
	if err != nil {
		a.initErr = err
		return
	}
	a.store = store
	go a.notifyLoop()
}

// notifyLoop sends the due-today summary at startup and then once at each
// following local midnight, so an app left running across days still surfaces
// each new day's workload (a single startup notification would otherwise be the
// only one a long-running session ever gets). It exits when the app context is
// cancelled on shutdown.
func (a *App) notifyLoop() {
	for {
		a.notifyDueToday()
		now := a.now()
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		timer := time.NewTimer(midnight.AddDate(0, 0, 1).Sub(now))
		select {
		case <-a.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// shutdown is called when the app exits; it closes the database handle so the
// WAL is checkpointed cleanly.
func (a *App) shutdown(ctx context.Context) {
	if a.store != nil {
		a.store.Close()
	}
}

// notifyDueToday sends a native notification summarising today's workload.
// Every step is best-effort: notifications are unavailable in unbundled dev
// builds and the user may decline authorization, neither of which should
// affect the app.
func (a *App) notifyDueToday() {
	a.store.mu.Lock()
	today := a.now().Format(dateLayout)
	due, overdue := 0, 0
	for _, t := range a.store.tasks {
		if t.Archived {
			continue
		}
		for _, s := range t.Sessions {
			if s.Done {
				continue
			}
			if s.Date == today {
				due++
			} else if s.Date < today {
				overdue++
			}
		}
	}
	a.store.mu.Unlock()
	if due == 0 && overdue == 0 {
		return
	}

	if !wruntime.IsNotificationAvailable(a.ctx) {
		return
	}
	if err := wruntime.InitializeNotifications(a.ctx); err != nil {
		return
	}
	authorized, err := wruntime.CheckNotificationAuthorization(a.ctx)
	if err != nil {
		return
	}
	if !authorized {
		if authorized, err = wruntime.RequestNotificationAuthorization(a.ctx); err != nil || !authorized {
			return
		}
	}

	var parts []string
	if due > 0 {
		parts = append(parts, fmt.Sprintf("%d session%s due today", due, plural(due)))
	}
	if overdue > 0 {
		parts = append(parts, fmt.Sprintf("%d overdue", overdue))
	}
	_ = wruntime.SendNotification(a.ctx, wruntime.NotificationOptions{
		ID:    "study-planner-due-today",
		Title: "Study Planner",
		Body:  strings.Join(parts, " · "),
	})
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ready reports whether the store loaded successfully.
func (a *App) ready() error {
	if a.store == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errors.New("study planner is not ready yet")
	}
	return nil
}

// snapshot returns a sorted, deep-copied view of the whole graph (subjects +
// tasks). The caller must hold the lock. Copying matters: Wails serializes the
// returned value after the lock is released, so handing out interior pointers
// would race with the next mutation.
func (a *App) snapshot() *State {
	for _, t := range a.store.tasks {
		sortSessions(t.Sessions)
	}
	sortTasks(a.store.tasks)
	sortSubjects(a.store.subjects)
	return &State{
		Subjects: cloneSubjects(a.store.subjects),
		Tasks:    cloneTasks(a.store.tasks),
		Settings: a.store.settings,
	}
}

// mutate runs fn under the store lock, persists, and returns the new snapshot.
// The graph is restored from a pre-mutation backup when fn or the save fails,
// so memory never drifts from disk: without the rollback, a failed save (or an
// fn erroring after partial changes) would leave the mutation live in memory,
// invisible to the frontend, and silently persisted by the next successful
// save.
func (a *App) mutate(fn func() error) (*State, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	backupTasks := cloneTasks(a.store.tasks)
	backupSubjects := cloneSubjects(a.store.subjects)
	restore := func() {
		a.store.tasks = backupTasks
		a.store.subjects = backupSubjects
	}
	if err := fn(); err != nil {
		restore()
		return nil, err
	}
	if err := a.store.save(); err != nil {
		restore()
		return nil, err
	}
	return a.snapshot(), nil
}

// mutateTask locates a task and applies fn to it.
func (a *App) mutateTask(id string, fn func(*Task) error) (*State, error) {
	return a.mutate(func() error {
		t := a.store.findTask(id)
		if t == nil {
			return errors.New("task not found")
		}
		return fn(t)
	})
}

// GetState returns the whole graph: all subjects and all tasks with their sessions.
func (a *App) GetState() (*State, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	return a.snapshot(), nil
}

// ExportCalendar writes the outstanding study schedule to an .ics file chosen by
// the user through a native save dialog, for importing into Google, Apple or
// Outlook calendars. It returns the path written, or "" if the user cancelled
// the dialog. The export is a point-in-time snapshot built under the store lock;
// re-export to reflect later schedule changes.
func (a *App) ExportCalendar() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	a.store.mu.Lock()
	ics := buildICS(a.store.tasks, a.now())
	a.store.mu.Unlock()

	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export study calendar",
		DefaultFilename: "study-planner.ics",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Calendar files (*.ics)", Pattern: "*.ics"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // user cancelled the dialog
	}
	if err := os.WriteFile(path, []byte(ics), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// SetDailyGoalMinutes sets the target amount of focus time (in minutes) to log
// each day, surfaced as the Home page's progress ring. A goal of 0 disables the
// target. It persists the single setting row rather than rewriting the graph.
func (a *App) SetDailyGoalMinutes(minutes int) (*State, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if minutes < 0 {
		return nil, errors.New("daily goal cannot be negative")
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if err := a.store.setDailyGoalMinutes(minutes); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// AddTask creates a new task. The name is required; the description is optional.
// subjectID assigns it to a subject ("" = ungrouped); a non-empty id must exist.
func (a *App) AddTask(name, description, subjectID string) (*State, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("task name is required")
	}
	return a.mutate(func() error {
		if subjectID != "" && a.store.findSubject(subjectID) == nil {
			return errors.New("subject not found")
		}
		a.store.tasks = append(a.store.tasks, &Task{
			ID:          uuid.NewString(),
			Name:        name,
			Description: strings.TrimSpace(description),
			Color:       pickColor(taskColors(a.store.tasks)),
			SubjectID:   subjectID,
			Tags:        []string{},
			Order:       len(a.store.tasks),
			CreatedAt:   a.now(),
			Sessions:    []*Session{},
		})
		return nil
	})
}

// UpdateTask edits an existing task's name, description and tags.
func (a *App) UpdateTask(id, name, description string, tags []string) (*State, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("task name is required")
	}
	return a.mutateTask(id, func(t *Task) error {
		t.Name = name
		t.Description = strings.TrimSpace(description)
		t.Tags = normalizeTags(tags)
		return nil
	})
}

// SetTaskColor sets a task's palette colour. An empty string resets it to the
// default accent; any other value must be a known palette token.
func (a *App) SetTaskColor(id, color string) (*State, error) {
	if !validColor(color) {
		return nil, errors.New("unknown colour")
	}
	return a.mutateTask(id, func(t *Task) error {
		t.Color = color
		return nil
	})
}

// SetTaskArchived archives or unarchives a task.
func (a *App) SetTaskArchived(id string, archived bool) (*State, error) {
	return a.mutateTask(id, func(t *Task) error {
		t.Archived = archived
		return nil
	})
}

// SetTaskSubject moves a task to a subject ("" = ungroup). A non-empty subjectID
// must reference an existing subject.
func (a *App) SetTaskSubject(taskID, subjectID string) (*State, error) {
	return a.mutate(func() error {
		t := a.store.findTask(taskID)
		if t == nil {
			return errors.New("task not found")
		}
		if subjectID != "" && a.store.findSubject(subjectID) == nil {
			return errors.New("subject not found")
		}
		t.SubjectID = subjectID
		return nil
	})
}

// ReorderTasks applies a new manual order. orderedIDs lists task ids in the
// desired order; any task not included keeps its relative order after them
// (e.g. tasks in other subject groups, or archived tasks hidden from the
// reorderable list).
func (a *App) ReorderTasks(orderedIDs []string) (*State, error) {
	return a.mutate(func() error {
		pos := make(map[string]int, len(orderedIDs))
		for i, id := range orderedIDs {
			pos[id] = i
		}
		// Establish the current relative order first so unlisted tasks keep it.
		sortTasks(a.store.tasks)
		next := len(orderedIDs)
		for _, t := range a.store.tasks {
			if p, ok := pos[t.ID]; ok {
				t.Order = p
			} else {
				t.Order = next
				next++
			}
		}
		normalizeOrder(a.store.tasks)
		return nil
	})
}

// DeleteTask removes a task and all of its sessions.
func (a *App) DeleteTask(id string) (*State, error) {
	return a.mutate(func() error {
		kept := slices.DeleteFunc(a.store.tasks, func(t *Task) bool { return t.ID == id })
		if len(kept) == len(a.store.tasks) {
			return errors.New("task not found")
		}
		a.store.tasks = kept
		normalizeOrder(a.store.tasks)
		return nil
	})
}

// AddSubject creates a new subject. The name is required.
func (a *App) AddSubject(name string) (*State, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("subject name is required")
	}
	return a.mutate(func() error {
		a.store.subjects = append(a.store.subjects, &Subject{
			ID:        uuid.NewString(),
			Name:      name,
			Color:     pickColor(subjectColors(a.store.subjects)),
			Order:     len(a.store.subjects),
			CreatedAt: a.now(),
		})
		return nil
	})
}

// mutateSubject locates a subject and applies fn to it.
func (a *App) mutateSubject(id string, fn func(*Subject) error) (*State, error) {
	return a.mutate(func() error {
		sub := a.store.findSubject(id)
		if sub == nil {
			return errors.New("subject not found")
		}
		return fn(sub)
	})
}

// UpdateSubject edits an existing subject's name.
func (a *App) UpdateSubject(id, name string) (*State, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("subject name is required")
	}
	return a.mutateSubject(id, func(sub *Subject) error {
		sub.Name = name
		return nil
	})
}

// SetSubjectColor sets a subject's palette colour. An empty string resets it to
// the default accent; any other value must be a known palette token.
func (a *App) SetSubjectColor(id, color string) (*State, error) {
	if !validColor(color) {
		return nil, errors.New("unknown colour")
	}
	return a.mutateSubject(id, func(sub *Subject) error {
		sub.Color = color
		return nil
	})
}

// ReorderSubjects applies a new manual order to subjects. Any subject not listed
// keeps its relative order after the listed ones — mirrors ReorderTasks.
func (a *App) ReorderSubjects(orderedIDs []string) (*State, error) {
	return a.mutate(func() error {
		pos := make(map[string]int, len(orderedIDs))
		for i, id := range orderedIDs {
			pos[id] = i
		}
		sortSubjects(a.store.subjects)
		next := len(orderedIDs)
		for _, sub := range a.store.subjects {
			if p, ok := pos[sub.ID]; ok {
				sub.Order = p
			} else {
				sub.Order = next
				next++
			}
		}
		normalizeSubjectOrder(a.store.subjects)
		return nil
	})
}

// DeleteSubject removes a subject and ungroups (does not delete) every task that
// belonged to it, so the tasks fall back to the "Ungrouped" section.
func (a *App) DeleteSubject(id string) (*State, error) {
	return a.mutate(func() error {
		kept := slices.DeleteFunc(a.store.subjects, func(sub *Subject) bool { return sub.ID == id })
		if len(kept) == len(a.store.subjects) {
			return errors.New("subject not found")
		}
		a.store.subjects = kept
		for _, t := range a.store.tasks {
			if t.SubjectID == id {
				t.SubjectID = ""
			}
		}
		normalizeSubjectOrder(a.store.subjects)
		return nil
	})
}

// AddSession adds a single, manually-chosen study date to a task.
func (a *App) AddSession(taskID, date string) (*State, error) {
	date = strings.TrimSpace(date)
	if _, err := time.Parse(dateLayout, date); err != nil {
		return nil, errors.New("date must be in YYYY-MM-DD format")
	}
	return a.mutateTask(taskID, func(t *Task) error {
		t.addDates([]string{date})
		return nil
	})
}

// AddSpacedSessions generates a task's spaced-repetition schedule from a
// start date and a set of day offsets. With replace=true any existing sessions
// (including their done state) are cleared first, so the result is exactly the
// new schedule; with replace=false the new dates are merged into the existing
// ones (dates the task already has are kept as-is, done state intact). The
// frontend asks the user which they want when sessions exist. If intervals is
// empty the default schedule (0, 1, 3, 7, 14, 30 days) is used.
func (a *App) AddSpacedSessions(taskID, startDate string, intervals []int, replace bool) (*State, error) {
	startDate = strings.TrimSpace(startDate)
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return nil, errors.New("start date must be in YYYY-MM-DD format")
	}
	if len(intervals) == 0 {
		intervals = DefaultIntervals
	}
	return a.mutateTask(taskID, func(t *Task) error {
		if replace {
			t.Sessions = []*Session{}
		}
		t.addDates(spacedDates(start, intervals))
		return nil
	})
}

// DeleteSession removes a single study date from a task.
func (a *App) DeleteSession(taskID, sessionID string) (*State, error) {
	return a.mutateTask(taskID, func(t *Task) error {
		if !t.removeSession(sessionID) {
			return errors.New("session not found")
		}
		return nil
	})
}

// ToggleSession flips the done state of a study session, stamping (or
// clearing) the completion time so stats can track when studying happened.
func (a *App) ToggleSession(taskID, sessionID string) (*State, error) {
	return a.mutateTask(taskID, func(t *Task) error {
		s := t.findSession(sessionID)
		if s == nil {
			return errors.New("session not found")
		}
		s.Done = !s.Done
		if s.Done {
			now := a.now()
			s.CompletedAt = &now
		} else {
			s.CompletedAt = nil
		}
		return nil
	})
}

// focusSnapshot returns a sorted (oldest first), deep-copied view of the focus
// log. The caller must hold the lock; copying keeps Wails from serializing
// interior pointers that the next append could race with.
func (a *App) focusSnapshot() []*FocusSession {
	out := make([]*FocusSession, len(a.store.focus))
	for i, f := range a.store.focus {
		c := *f
		out[i] = &c
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CompletedAt.Before(out[j].CompletedAt)
	})
	return out
}

// GetFocusSessions returns the whole completed-focus-block log.
func (a *App) GetFocusSessions() ([]*FocusSession, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	return a.focusSnapshot(), nil
}

// RecordFocusSession logs a focus block of durationSec seconds against taskID
// ("" for general focus, otherwise an existing task) and returns the full focus
// log so the frontend can replace its state in one go. The frontend sends both
// blocks that ran to completion and the partial time from blocks ended early
// (above a minimum it enforces); fully-abandoned attempts never reach here.
func (a *App) RecordFocusSession(taskID string, durationSec int) ([]*FocusSession, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if durationSec <= 0 {
		return nil, errors.New("focus duration must be positive")
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if taskID != "" && a.store.findTask(taskID) == nil {
		return nil, errors.New("task not found")
	}
	if err := a.store.addFocusSession(&FocusSession{
		ID:          uuid.NewString(),
		TaskID:      taskID,
		DurationSec: durationSec,
		CompletedAt: a.now(),
	}); err != nil {
		return nil, err
	}
	return a.focusSnapshot(), nil
}

// SetTaskAdaptive enables or disables grade-based rescheduling for a task.
func (a *App) SetTaskAdaptive(id string, adaptive bool) (*State, error) {
	return a.mutateTask(id, func(t *Task) error {
		t.Adaptive = adaptive
		return nil
	})
}

// RescheduleSession moves a session to a new date. If the task already has a
// session on that date the moved one is dropped instead of duplicating the day,
// matching the one-session-per-day rule used everywhere else.
func (a *App) RescheduleSession(taskID, sessionID, date string) (*State, error) {
	date = strings.TrimSpace(date)
	if _, err := time.Parse(dateLayout, date); err != nil {
		return nil, errors.New("date must be in YYYY-MM-DD format")
	}
	return a.mutateTask(taskID, func(t *Task) error {
		target := t.findSession(sessionID)
		if target == nil {
			return errors.New("session not found")
		}
		if target.Date == date {
			return nil
		}
		// A pending session already on that day makes the move redundant, so
		// drop the moved one (merge). A done session there is historical and
		// doesn't block — the moved review coexists with it.
		if t.hasPendingOn(date) {
			t.removeSession(sessionID)
			return nil
		}
		target.Date = date
		return nil
	})
}

// RescheduleOverdueSessions moves every overdue, not-done session of every
// active task to today — the agenda's one-click catch-up. A task ends up with
// at most one pending session today; surplus overdue ones are removed as
// covered. A done session already on today doesn't count as covering: the
// overdue review still moves to today and coexists with it.
func (a *App) RescheduleOverdueSessions() (*State, error) {
	return a.mutate(func() error {
		today := a.now().Format(dateLayout)
		for _, t := range a.store.tasks {
			if t.Archived {
				continue
			}
			hasToday := t.hasPendingOn(today)
			kept := t.Sessions[:0]
			for _, s := range t.Sessions {
				if !s.Done && s.Date < today {
					if hasToday {
						continue
					}
					s.Date = today
					hasToday = true
				}
				kept = append(kept, s)
			}
			t.Sessions = kept
		}
		return nil
	})
}

// gradeFactors scales the gaps between today and each remaining review.
// "again" additionally forces the next review to tomorrow.
var gradeFactors = map[string]float64{
	"again": 0.5,
	"hard":  0.7,
	"good":  1.0,
	"easy":  1.4,
}

// GradeSession marks a session done with a recall grade and re-spaces the
// task's remaining schedule (SM-2 lite). Gaps from the graded session to each
// later not-done session are scaled by the grade's factor and re-anchored to
// today, so overdue schedules also catch up. Sessions scheduled before the
// graded one are left alone. Grading "again" with nothing left schedules one
// review for tomorrow.
func (a *App) GradeSession(taskID, sessionID, grade string) (*State, error) {
	factor, ok := gradeFactors[grade]
	if !ok {
		return nil, errors.New("grade must be one of: again, hard, good, easy")
	}
	return a.mutateTask(taskID, func(t *Task) error {
		graded := t.findSession(sessionID)
		if graded == nil {
			return errors.New("session not found")
		}
		if graded.Done {
			return errors.New("session is already done")
		}
		// Validate before mutating: a malformed date (possible via the legacy
		// JSON import) must not leave the session half-graded.
		gradedDate, err := time.Parse(dateLayout, graded.Date)
		if err != nil {
			return err
		}
		now := a.now()
		graded.Done = true
		graded.CompletedAt = &now
		// Only pending sessions occupy a day (pendingDates) — done sessions are
		// historical, so the re-spaced reviews may land on (and coexist with) a
		// completed day; notably, grading "again" can still schedule tomorrow
		// even if an early review was already completed there. Sessions after
		// the graded one are about to be rewritten, so their old dates leave
		// the collision set.
		occupied := t.pendingDates()
		var future []*Session
		for _, s := range t.Sessions {
			if !s.Done && s.Date > graded.Date {
				future = append(future, s)
				delete(occupied, s.Date)
			}
		}
		sort.SliceStable(future, func(i, j int) bool { return future[i].Date < future[j].Date })

		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		dateAt := func(n int) string { return today.AddDate(0, 0, n).Format(dateLayout) }
		prev := 0
		for _, s := range future {
			d, err := time.Parse(dateLayout, s.Date)
			if err != nil {
				continue
			}
			gap := int(d.Sub(gradedDate).Hours() / 24)
			next := int(math.Round(float64(gap) * factor))
			if grade == "again" && prev == 0 {
				next = 1
			}
			if next <= prev {
				next = prev + 1
			}
			for occupied[dateAt(next)] {
				next++
			}
			s.Date = dateAt(next)
			occupied[s.Date] = true
			prev = next
		}
		if grade == "again" && len(future) == 0 && !occupied[dateAt(1)] {
			t.Sessions = append(t.Sessions, &Session{ID: uuid.NewString(), Date: dateAt(1)})
		}
		return nil
	})
}
