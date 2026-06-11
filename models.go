package main

import (
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
	Order       int        `json:"order"` // manual sort position
	CreatedAt   time.Time  `json:"createdAt"`
	Sessions    []*Session `json:"sessions"`
}

// TopicColors are the palette tokens a topic may use; the frontend maps each to a
// concrete colour. New topics cycle through this list so they start out distinct.
var TopicColors = []string{"blue", "violet", "emerald", "amber", "rose", "cyan", "orange", "slate"}

// validColor reports whether c is a known palette token. The empty string is
// allowed and means "use the default accent".
func validColor(c string) bool {
	if c == "" {
		return true
	}
	for _, t := range TopicColors {
		if t == c {
			return true
		}
	}
	return false
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

// normalizeOrder sorts topics by their existing Order (creation time breaks
// ties) and reassigns a contiguous 0..n-1 Order. This migrates legacy data
// (all-zero Order falls back to creation order) and compacts gaps left by deletes.
func normalizeOrder(topics []*Topic) {
	sort.SliceStable(topics, func(i, j int) bool {
		if topics[i].Order != topics[j].Order {
			return topics[i].Order < topics[j].Order
		}
		return topics[i].CreatedAt.Before(topics[j].CreatedAt)
	})
	for i, t := range topics {
		t.Order = i
	}
}

// Session is a single planned study date for a topic.
type Session struct {
	ID   string `json:"id"`
	Date string `json:"date"` // YYYY-MM-DD
	Done bool   `json:"done"`
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
