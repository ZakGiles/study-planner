package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound application. Every mutating method returns the full,
// freshly-sorted list of topics so the frontend can replace its state in one go.
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
	go a.notifyDueToday()
}

// notifyDueToday sends a native notification summarising today's workload.
// Every step is best-effort: notifications are unavailable in unbundled dev
// builds and the user may decline authorization, neither of which should
// affect the app.
func (a *App) notifyDueToday() {
	a.store.mu.Lock()
	today := a.now().Format(dateLayout)
	due, overdue := 0, 0
	for _, t := range a.store.topics {
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

// snapshot returns a sorted, deep-copied view of all topics. The caller must
// hold the lock. Copying matters: Wails serializes the returned value after
// the lock is released, so handing out interior pointers would race with the
// next mutation.
func (a *App) snapshot() []*Topic {
	topics := a.store.topics
	for _, t := range topics {
		sortSessions(t.Sessions)
	}
	sort.SliceStable(topics, func(i, j int) bool {
		if topics[i].Order != topics[j].Order {
			return topics[i].Order < topics[j].Order
		}
		return topics[i].CreatedAt.Before(topics[j].CreatedAt)
	})
	out := make([]*Topic, len(topics))
	for i, t := range topics {
		c := *t
		c.Tags = make([]string, len(t.Tags))
		copy(c.Tags, t.Tags)
		c.Sessions = make([]*Session, len(t.Sessions))
		for j, s := range t.Sessions {
			sc := *s
			c.Sessions[j] = &sc
		}
		out[i] = &c
	}
	return out
}

// mutate runs fn under the store lock, persists, and returns the new
// snapshot. fn returning an error skips the save.
func (a *App) mutate(fn func() error) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if err := fn(); err != nil {
		return nil, err
	}
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// mutateTopic locates a topic and applies fn to it.
func (a *App) mutateTopic(id string, fn func(*Topic) error) ([]*Topic, error) {
	return a.mutate(func() error {
		t := a.store.find(id)
		if t == nil {
			return errors.New("topic not found")
		}
		return fn(t)
	})
}

// GetTopics returns all topics with their sessions.
func (a *App) GetTopics() ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	return a.snapshot(), nil
}

// AddTopic creates a new topic. The name is required; the description is optional.
func (a *App) AddTopic(name, description string) ([]*Topic, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("topic name is required")
	}
	return a.mutate(func() error {
		a.store.topics = append(a.store.topics, &Topic{
			ID:          uuid.NewString(),
			Name:        name,
			Description: strings.TrimSpace(description),
			Color:       TopicColors[len(a.store.topics)%len(TopicColors)],
			Tags:        []string{},
			Order:       len(a.store.topics),
			CreatedAt:   a.now(),
			Sessions:    []*Session{},
		})
		return nil
	})
}

// UpdateTopic edits an existing topic's name, description and tags.
func (a *App) UpdateTopic(id, name, description string, tags []string) ([]*Topic, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("topic name is required")
	}
	return a.mutateTopic(id, func(t *Topic) error {
		t.Name = name
		t.Description = strings.TrimSpace(description)
		t.Tags = normalizeTags(tags)
		return nil
	})
}

// SetTopicColor sets a topic's palette colour. An empty string resets it to the
// default accent; any other value must be a known palette token.
func (a *App) SetTopicColor(id, color string) ([]*Topic, error) {
	if !validColor(color) {
		return nil, errors.New("unknown colour")
	}
	return a.mutateTopic(id, func(t *Topic) error {
		t.Color = color
		return nil
	})
}

// SetTopicArchived archives or unarchives a topic.
func (a *App) SetTopicArchived(id string, archived bool) ([]*Topic, error) {
	return a.mutateTopic(id, func(t *Topic) error {
		t.Archived = archived
		return nil
	})
}

// ReorderTopics applies a new manual order. orderedIDs lists topic ids in the
// desired order; any topic not included keeps its relative order after them
// (e.g. archived topics that are hidden from the reorderable list).
func (a *App) ReorderTopics(orderedIDs []string) ([]*Topic, error) {
	return a.mutate(func() error {
		pos := make(map[string]int, len(orderedIDs))
		for i, id := range orderedIDs {
			pos[id] = i
		}
		// Establish the current relative order first so unlisted topics keep it.
		sort.SliceStable(a.store.topics, func(i, j int) bool {
			return a.store.topics[i].Order < a.store.topics[j].Order
		})
		next := len(orderedIDs)
		for _, t := range a.store.topics {
			if p, ok := pos[t.ID]; ok {
				t.Order = p
			} else {
				t.Order = next
				next++
			}
		}
		normalizeOrder(a.store.topics)
		return nil
	})
}

// DeleteTopic removes a topic and all of its sessions.
func (a *App) DeleteTopic(id string) ([]*Topic, error) {
	return a.mutate(func() error {
		kept := a.store.topics[:0]
		found := false
		for _, t := range a.store.topics {
			if t.ID == id {
				found = true
				continue
			}
			kept = append(kept, t)
		}
		if !found {
			return errors.New("topic not found")
		}
		a.store.topics = kept
		normalizeOrder(a.store.topics)
		return nil
	})
}

// AddSession adds a single, manually-chosen study date to a topic.
func (a *App) AddSession(topicID, date string) ([]*Topic, error) {
	date = strings.TrimSpace(date)
	if _, err := time.Parse(dateLayout, date); err != nil {
		return nil, errors.New("date must be in YYYY-MM-DD format")
	}
	return a.mutateTopic(topicID, func(t *Topic) error {
		t.addDates([]string{date})
		return nil
	})
}

// AddSpacedSessions generates a topic's spaced-repetition schedule from a
// start date and a set of day offsets. With replace=true any existing sessions
// (including their done state) are cleared first, so the result is exactly the
// new schedule; with replace=false the new dates are merged into the existing
// ones (dates the topic already has are kept as-is, done state intact). The
// frontend asks the user which they want when sessions exist. If intervals is
// empty the default schedule (0, 1, 3, 7, 14, 30 days) is used.
func (a *App) AddSpacedSessions(topicID, startDate string, intervals []int, replace bool) ([]*Topic, error) {
	startDate = strings.TrimSpace(startDate)
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return nil, errors.New("start date must be in YYYY-MM-DD format")
	}
	if len(intervals) == 0 {
		intervals = DefaultIntervals
	}
	return a.mutateTopic(topicID, func(t *Topic) error {
		if replace {
			t.Sessions = []*Session{}
		}
		t.addDates(spacedDates(start, intervals))
		return nil
	})
}

// DeleteSession removes a single study date from a topic.
func (a *App) DeleteSession(topicID, sessionID string) ([]*Topic, error) {
	return a.mutateTopic(topicID, func(t *Topic) error {
		if !t.removeSession(sessionID) {
			return errors.New("session not found")
		}
		return nil
	})
}

// ToggleSession flips the done state of a study session, stamping (or
// clearing) the completion time so stats can track when studying happened.
func (a *App) ToggleSession(topicID, sessionID string) ([]*Topic, error) {
	return a.mutateTopic(topicID, func(t *Topic) error {
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

// SetTopicAdaptive enables or disables grade-based rescheduling for a topic.
func (a *App) SetTopicAdaptive(id string, adaptive bool) ([]*Topic, error) {
	return a.mutateTopic(id, func(t *Topic) error {
		t.Adaptive = adaptive
		return nil
	})
}

// RescheduleSession moves a session to a new date. If the topic already has a
// session on that date the moved one is dropped instead of duplicating the day,
// matching the one-session-per-day rule used everywhere else.
func (a *App) RescheduleSession(topicID, sessionID, date string) ([]*Topic, error) {
	date = strings.TrimSpace(date)
	if _, err := time.Parse(dateLayout, date); err != nil {
		return nil, errors.New("date must be in YYYY-MM-DD format")
	}
	return a.mutateTopic(topicID, func(t *Topic) error {
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
// active topic to today — the agenda's one-click catch-up. A topic ends up with
// at most one pending session today; surplus overdue ones are removed as
// covered. A done session already on today doesn't count as covering: the
// overdue review still moves to today and coexists with it.
func (a *App) RescheduleOverdueSessions() ([]*Topic, error) {
	return a.mutate(func() error {
		today := a.now().Format(dateLayout)
		for _, t := range a.store.topics {
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
// topic's remaining schedule (SM-2 lite). Gaps from the graded session to each
// later not-done session are scaled by the grade's factor and re-anchored to
// today, so overdue schedules also catch up. Sessions scheduled before the
// graded one are left alone. Grading "again" with nothing left schedules one
// review for tomorrow.
func (a *App) GradeSession(topicID, sessionID, grade string) ([]*Topic, error) {
	factor, ok := gradeFactors[grade]
	if !ok {
		return nil, errors.New("grade must be one of: again, hard, good, easy")
	}
	return a.mutateTopic(topicID, func(t *Topic) error {
		graded := t.findSession(sessionID)
		if graded == nil {
			return errors.New("session not found")
		}
		if graded.Done {
			return errors.New("session is already done")
		}
		now := a.now()
		graded.Done = true
		graded.CompletedAt = &now

		gradedDate, err := time.Parse(dateLayout, graded.Date)
		if err != nil {
			return err
		}
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
			for occupied[today.AddDate(0, 0, next).Format(dateLayout)] {
				next++
			}
			s.Date = today.AddDate(0, 0, next).Format(dateLayout)
			occupied[s.Date] = true
			prev = next
		}
		if grade == "again" && len(future) == 0 {
			tomorrow := today.AddDate(0, 0, 1).Format(dateLayout)
			if !occupied[tomorrow] {
				t.Sessions = append(t.Sessions, &Session{ID: uuid.NewString(), Date: tomorrow})
			}
		}
		return nil
	})
}
