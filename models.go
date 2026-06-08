package main

import (
	"sort"
	"time"
)

// dateLayout is the canonical format used for all study dates (no time component).
const dateLayout = "2006-01-02"

// Topic is a subject the user wants to study, together with its scheduled
// study sessions.
type Topic struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"createdAt"`
	Sessions    []*Session `json:"sessions"`
}

// Session is a single planned study date for a topic.
type Session struct {
	ID   string `json:"id"`
	Date string `json:"date"` // YYYY-MM-DD
	Done bool   `json:"done"`
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
