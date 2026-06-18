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

// Topic is a subject the user wants to study, together with its scheduled
// study sessions.
type Topic struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Color       string     `json:"color"` // palette token; "" = default
	Tags        []string   `json:"tags"`
	Archived    bool       `json:"archived"`
	Adaptive    bool       `json:"adaptive"` // grade reviews and re-space the schedule
	Order       int        `json:"order"`    // manual sort position
	CreatedAt   time.Time  `json:"createdAt"`
	Sessions    []*Session `json:"sessions"`
}

// TopicColors are the palette tokens a topic may use; the frontend maps each to a
// concrete colour. New topics cycle through this list so they start out distinct.
var TopicColors = []string{"blue", "violet", "emerald", "amber", "rose", "cyan", "orange", "slate"}

// pickColor returns the palette token least used by the existing topics,
// breaking ties by the palette's natural order. For sequential adds with no
// deletions this reproduces the plain round-robin cycle (blue, violet, …); once
// deletions have unbalanced the counts it still hands new topics a distinct
// colour rather than blindly repeating one. Topics with a reset ("") colour
// don't count against any token.
func pickColor(topics []*Topic) string {
	counts := make(map[string]int, len(TopicColors))
	for _, t := range topics {
		counts[t.Color]++
	}
	best := TopicColors[0]
	for _, c := range TopicColors[1:] {
		if counts[c] < counts[best] {
			best = c
		}
	}
	return best
}

// validColor reports whether c is a known palette token. The empty string is
// allowed and means "use the default accent".
func validColor(c string) bool {
	return c == "" || slices.Contains(TopicColors, c)
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

// sortTopics orders topics by their manual Order, breaking ties (e.g. legacy
// all-zero data) by creation time.
func sortTopics(topics []*Topic) {
	sort.SliceStable(topics, func(i, j int) bool {
		if topics[i].Order != topics[j].Order {
			return topics[i].Order < topics[j].Order
		}
		return topics[i].CreatedAt.Before(topics[j].CreatedAt)
	})
}

// normalizeOrder sorts topics and reassigns a contiguous 0..n-1 Order. This
// migrates legacy data (all-zero Order falls back to creation order) and
// compacts gaps left by deletes.
func normalizeOrder(topics []*Topic) {
	sortTopics(topics)
	for i, t := range topics {
		t.Order = i
	}
}

// FocusSession is a completed focus block from the Pomodoro-style focus timer.
// TopicID is the topic the user focused on, or "" for general focus not tied to
// a topic. Unlike sessions, focus records carry no SQL foreign key to topics:
// the store's whole-graph save() rewrites the topics table on every mutation,
// which would cascade onto focus history; keeping topic_id a plain string lets
// the focus log persist independently and survive topic edits and deletes. Only
// completed focus blocks are recorded — abandoned or partial time is not.
type FocusSession struct {
	ID          string    `json:"id"`
	TopicID     string    `json:"topicId"` // "" = general focus
	DurationSec int       `json:"durationSec"`
	CompletedAt time.Time `json:"completedAt"`
}

// Session is a single planned study date for a topic. CompletedAt records when
// it was actually checked off (nil while not done; legacy done sessions from
// before this field also have nil, and consumers fall back to Date).
type Session struct {
	ID          string     `json:"id"`
	Date        string     `json:"date"` // YYYY-MM-DD
	Done        bool       `json:"done"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// hasPendingOn reports whether the topic has a not-done session on date. Done
// sessions are historical records and never block scheduling a new review, so
// rescheduling and grading treat a day as free unless a pending session sits on
// it. (addDates, by contrast, dedupes against all dates: generating a schedule
// should not re-add a day already completed.)
func (t *Topic) hasPendingOn(date string) bool {
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
func (t *Topic) pendingDates() map[string]bool {
	m := make(map[string]bool, len(t.Sessions))
	for _, s := range t.Sessions {
		if !s.Done {
			m[s.Date] = true
		}
	}
	return m
}

// findSession returns the session with the given id, or nil.
func (t *Topic) findSession(id string) *Session {
	for _, s := range t.Sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// removeSession drops the session with the given id, returning whether it was
// found. The caller must hold the store lock.
func (t *Topic) removeSession(id string) bool {
	for i, s := range t.Sessions {
		if s.ID == id {
			t.Sessions = append(t.Sessions[:i], t.Sessions[i+1:]...)
			return true
		}
	}
	return false
}

// addDates appends new sessions for any dates the topic does not already have.
// The caller must hold the store lock.
func (t *Topic) addDates(dates []string) {
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

// sortSessions orders a topic's sessions chronologically by date.
func sortSessions(sessions []*Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Date < sessions[j].Date
	})
}
