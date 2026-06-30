package main

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// dateLayout is the canonical format used for all study dates (no time component).
const dateLayout = "2006-01-02"

// Task is something the user wants to study, together with its scheduled study
// sessions. A task optionally belongs to a Subject (SubjectID == "" = ungrouped).
type Task struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Color       string     `json:"color"`     // palette token; "" = default
	SubjectID   string     `json:"subjectId"` // owning subject; "" = ungrouped
	Tags        []string   `json:"tags"`
	Archived    bool       `json:"archived"`
	Adaptive    bool       `json:"adaptive"` // grade reviews and re-space the schedule
	Order       int        `json:"order"`    // manual sort position
	CreatedAt   time.Time  `json:"createdAt"`
	Sessions    []*Session `json:"sessions"`
}

// Subject is a first-class grouping of tasks (e.g. "Mathematics" holding the
// "Linear Algebra" and "Calculus" tasks). Subjects carry their own colour and
// manual sort order and may exist while empty.
type Subject struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"` // palette token; "" = default
	Order     int       `json:"order"` // manual sort position
	CreatedAt time.Time `json:"createdAt"`
}

// Settings holds user preferences that live with the data rather than as
// machine-local UI chrome. DailyGoalMinutes is the target amount of focus time
// (in minutes) to log each day, surfaced as the Home page's progress ring.
type Settings struct {
	DailyGoalMinutes int `json:"dailyGoalMinutes"`
}

// defaultDailyGoalMinutes is the focus goal a fresh database starts with (2h), so
// the Home ring is meaningful before the user ever opens settings.
const defaultDailyGoalMinutes = 120

// TaskColors are the palette tokens a task or subject may use; the frontend maps
// each to a concrete colour. New tasks cycle through this list so they start out
// distinct.
var TaskColors = []string{"blue", "violet", "emerald", "amber", "rose", "cyan", "orange", "slate"}

// pickColor returns the palette token least used among the supplied colours,
// breaking ties by the palette's natural order. For sequential adds with no
// deletions this reproduces the plain round-robin cycle (blue, violet, …); once
// deletions have unbalanced the counts it still hands the new item a distinct
// colour rather than blindly repeating one. A reset ("") colour doesn't count
// against any token.
func pickColor(used []string) string {
	counts := make(map[string]int, len(TaskColors))
	for _, c := range used {
		counts[c]++
	}
	best := TaskColors[0]
	for _, c := range TaskColors[1:] {
		if counts[c] < counts[best] {
			best = c
		}
	}
	return best
}

// taskColors and subjectColors collect the in-use palette tokens of each, for
// pickColor.
func taskColors(tasks []*Task) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.Color
	}
	return out
}

func subjectColors(subjects []*Subject) []string {
	out := make([]string, len(subjects))
	for i, s := range subjects {
		out[i] = s.Color
	}
	return out
}

// validColor reports whether c is a known palette token. The empty string is
// allowed and means "use the default accent".
func validColor(c string) bool {
	return c == "" || slices.Contains(TaskColors, c)
}

// normalizeTags trims, drops empties and de-duplicates tags case-insensitively
// (keeping the first-seen casing), capping both the count and each tag's length.
func normalizeTags(tags []string) []string {
	const maxTags, maxLen = 12, 30
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if r := []rune(t); len(r) > maxLen {
			t = string(r[:maxLen])
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
		if len(out) >= maxTags {
			break
		}
	}
	return out
}

// sortTasks orders tasks by their manual Order, breaking ties (e.g. legacy
// all-zero data) by creation time.
func sortTasks(tasks []*Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Order != tasks[j].Order {
			return tasks[i].Order < tasks[j].Order
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
}

// normalizeOrder sorts tasks and reassigns a contiguous 0..n-1 Order. This
// migrates legacy data (all-zero Order falls back to creation order) and
// compacts gaps left by deletes.
func normalizeOrder(tasks []*Task) {
	sortTasks(tasks)
	for i, t := range tasks {
		t.Order = i
	}
}

// sortSubjects orders subjects by their manual Order, breaking ties by creation
// time — mirrors sortTasks.
func sortSubjects(subjects []*Subject) {
	sort.SliceStable(subjects, func(i, j int) bool {
		if subjects[i].Order != subjects[j].Order {
			return subjects[i].Order < subjects[j].Order
		}
		return subjects[i].CreatedAt.Before(subjects[j].CreatedAt)
	})
}

// normalizeSubjectOrder sorts subjects and reassigns a contiguous 0..n-1 Order.
func normalizeSubjectOrder(subjects []*Subject) {
	sortSubjects(subjects)
	for i, s := range subjects {
		s.Order = i
	}
}

// FocusSession is a completed focus block from the Pomodoro-style focus timer.
// TaskID is the task the user focused on, or "" for general focus not tied to a
// task. Unlike sessions, focus records carry no SQL foreign key to tasks: the
// store's whole-graph save() rewrites the tasks table on every mutation, which
// would cascade onto focus history; keeping task_id a plain string lets the
// focus log persist independently and survive task edits and deletes. Both
// completed blocks and the partial time from blocks ended early are recorded;
// only fully-abandoned attempts (below the frontend's minimum) are not.
type FocusSession struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"taskId"` // "" = general focus
	DurationSec int       `json:"durationSec"`
	CompletedAt time.Time `json:"completedAt"`
}

// Session is a single planned study date for a task. CompletedAt records when
// it was actually checked off (nil while not done; legacy done sessions from
// before this field also have nil, and consumers fall back to Date).
type Session struct {
	ID          string     `json:"id"`
	Date        string     `json:"date"` // YYYY-MM-DD
	Done        bool       `json:"done"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// hasPendingOn reports whether the task has a not-done session on date. Done
// sessions are historical records and never block scheduling a new review, so
// rescheduling and grading treat a day as free unless a pending session sits on
// it. (addDates, by contrast, dedupes against all dates: generating a schedule
// should not re-add a day already completed.)
func (t *Task) hasPendingOn(date string) bool {
	for _, s := range t.Sessions {
		if !s.Done && s.Date == date {
			return true
		}
	}
	return false
}

// pendingDates returns the set of dates holding a not-done session — the
// domain of the one-pending-session-per-day invariant that hasPendingOn checks
// pointwise. Scheduling code that places multiple dates seeds its collision
// set from this.
func (t *Task) pendingDates() map[string]bool {
	m := make(map[string]bool, len(t.Sessions))
	for _, s := range t.Sessions {
		if !s.Done {
			m[s.Date] = true
		}
	}
	return m
}

// findSession returns the session with the given id, or nil.
func (t *Task) findSession(id string) *Session {
	for _, s := range t.Sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// removeSession drops the session with the given id, returning whether it was
// found. The caller must hold the store lock.
func (t *Task) removeSession(id string) bool {
	for i, s := range t.Sessions {
		if s.ID == id {
			t.Sessions = append(t.Sessions[:i], t.Sessions[i+1:]...)
			return true
		}
	}
	return false
}

// addDates appends new sessions for any dates the task does not already have.
// The caller must hold the store lock.
func (t *Task) addDates(dates []string) {
	existing := make(map[string]struct{}, len(t.Sessions))
	for _, s := range t.Sessions {
		existing[s.Date] = struct{}{}
	}
	for _, d := range dates {
		if _, ok := existing[d]; ok {
			continue
		}
		existing[d] = struct{}{}
		t.Sessions = append(t.Sessions, &Session{
			ID:   uuid.NewString(),
			Date: d,
		})
	}
}

// DefaultIntervals are day offsets from a start date that approximate a classic
// spaced-repetition schedule (study now, then after 1, 3, 7, 14 and 30 days).
var DefaultIntervals = []int{0, 1, 3, 7, 14, 30}

// spacedDates turns a start date and a set of day offsets into sorted,
// de-duplicated YYYY-MM-DD strings.
func spacedDates(start time.Time, intervals []int) []string {
	seen := make(map[string]struct{}, len(intervals))
	dates := make([]string, 0, len(intervals))
	for _, d := range intervals {
		if d < 0 {
			continue
		}
		date := start.AddDate(0, 0, d).Format(dateLayout)
		if _, ok := seen[date]; ok {
			continue
		}
		seen[date] = struct{}{}
		dates = append(dates, date)
	}
	sort.Strings(dates)
	return dates
}

// sortSessions orders a task's sessions chronologically by date.
func sortSessions(sessions []*Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Date < sessions[j].Date
	})
}
