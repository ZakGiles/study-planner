package main

import (
	"strings"
	"testing"
	"time"
)

// icalNow is a fixed instant used to make DTSTAMP deterministic. It is given in a
// non-UTC zone to confirm buildICS converts the stamp to UTC.
var icalNow = time.Date(2026, 6, 12, 12, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))

// lines splits an ICS document into its logical CRLF-separated lines and asserts
// CRLF endings were used throughout.
func lines(t *testing.T, ics string) []string {
	t.Helper()
	if strings.Contains(strings.ReplaceAll(ics, "\r\n", ""), "\n") {
		t.Fatalf("found a bare LF not part of a CRLF pair:\n%q", ics)
	}
	return strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n")
}

func countLine(ls []string, want string) int {
	n := 0
	for _, l := range ls {
		if l == want {
			n++
		}
	}
	return n
}

func hasLine(ls []string, want string) bool {
	return countLine(ls, want) > 0
}

func TestBuildICSStructureAndEvents(t *testing.T) {
	tasks := []*Task{
		{
			Name:        "Linear Algebra",
			Description: "Eigenvalues",
			Sessions: []*Session{
				{ID: "s1", Date: "2026-06-14"},
				{ID: "s2", Date: "2026-06-20", Done: true}, // done: excluded
			},
		},
		{
			Name:     "Archived",
			Archived: true,
			Sessions: []*Session{{ID: "s3", Date: "2026-06-15"}}, // archived: excluded
		},
		{
			Name:     "History",
			Sessions: []*Session{{ID: "s4", Date: "2026-06-13"}}, // no description
		},
	}

	ics := buildICS(tasks, icalNow)
	ls := lines(t, ics)

	// Calendar wrapper.
	if ls[0] != "BEGIN:VCALENDAR" || ls[len(ls)-1] != "END:VCALENDAR" {
		t.Fatalf("missing VCALENDAR wrapper:\n%s", ics)
	}
	for _, want := range []string{"VERSION:2.0", "PRODID:" + iCalProdID, "CALSCALE:GREGORIAN", "X-WR-CALNAME:Study Planner"} {
		if !hasLine(ls, want) {
			t.Errorf("missing header line %q", want)
		}
	}

	// Exactly two events: the pending sessions s1 and s4 (done + archived dropped).
	if got := countLine(ls, "BEGIN:VEVENT"); got != 2 {
		t.Fatalf("got %d VEVENTs, want 2:\n%s", got, ics)
	}
	for _, id := range []string{"s1", "s4"} {
		if !hasLine(ls, "UID:"+id+"@study-planner") {
			t.Errorf("missing UID for %q", id)
		}
	}
	for _, id := range []string{"s2", "s3"} {
		if hasLine(ls, "UID:"+id+"@study-planner") {
			t.Errorf("UID for excluded session %q should not appear", id)
		}
	}

	// DTSTAMP is the supplied instant converted to UTC (12:00 in UTC+2 → 10:00Z).
	if !hasLine(ls, "DTSTAMP:20260612T100000Z") {
		t.Errorf("DTSTAMP not converted to UTC:\n%s", ics)
	}

	// All-day event: DTEND is the day after DTSTART.
	if !hasLine(ls, "DTSTART;VALUE=DATE:20260614") || !hasLine(ls, "DTEND;VALUE=DATE:20260615") {
		t.Errorf("all-day DTSTART/DTEND wrong:\n%s", ics)
	}
	if !hasLine(ls, "SUMMARY:Study: Linear Algebra") {
		t.Errorf("missing summary:\n%s", ics)
	}
	if !hasLine(ls, "DESCRIPTION:Eigenvalues") {
		t.Errorf("missing description:\n%s", ics)
	}
}

func TestBuildICSSortedByDate(t *testing.T) {
	tasks := []*Task{{
		Name: "T",
		Sessions: []*Session{
			{ID: "late", Date: "2026-07-01"},
			{ID: "early", Date: "2026-06-01"},
		},
	}}
	ls := lines(t, buildICS(tasks, icalNow))
	var order []string
	for _, l := range ls {
		if strings.HasPrefix(l, "UID:") {
			order = append(order, l)
		}
	}
	if len(order) != 2 || order[0] != "UID:early@study-planner" {
		t.Fatalf("events not date-sorted: %v", order)
	}
}

func TestBuildICSEscaping(t *testing.T) {
	tasks := []*Task{{
		Name:        "Maths, Physics; Chemistry\\Bio",
		Description: "line one\nline two",
		Sessions:    []*Session{{ID: "s1", Date: "2026-06-14"}},
	}}
	ics := buildICS(tasks, icalNow)
	if !strings.Contains(ics, `SUMMARY:Study: Maths\, Physics\; Chemistry\\Bio`) {
		t.Errorf("summary special chars not escaped:\n%s", ics)
	}
	if !strings.Contains(ics, `DESCRIPTION:line one\nline two`) {
		t.Errorf("newline in description not escaped:\n%s", ics)
	}
}

func TestBuildICSFoldsLongLines(t *testing.T) {
	long := strings.Repeat("A", 200)
	tasks := []*Task{{
		Name:     long,
		Sessions: []*Session{{ID: "s1", Date: "2026-06-14"}},
	}}
	for _, l := range lines(t, buildICS(tasks, icalNow)) {
		// Continuation lines begin with a space; every physical line must stay
		// within the 75-octet limit.
		if len(l) > 75 {
			t.Fatalf("line exceeds 75 octets (%d): %q", len(l), l)
		}
	}
}

func TestBuildICSSkipsMalformedDate(t *testing.T) {
	tasks := []*Task{{
		Name: "T",
		Sessions: []*Session{
			{ID: "bad", Date: "not-a-date"},
			{ID: "ok", Date: "2026-06-14"},
		},
	}}
	ls := lines(t, buildICS(tasks, icalNow))
	if got := countLine(ls, "BEGIN:VEVENT"); got != 1 {
		t.Fatalf("got %d events, want 1 (malformed date skipped)", got)
	}
	if !hasLine(ls, "UID:ok@study-planner") {
		t.Error("valid session missing")
	}
}

func TestBuildICSEmpty(t *testing.T) {
	ls := lines(t, buildICS(nil, icalNow))
	if got := countLine(ls, "BEGIN:VEVENT"); got != 0 {
		t.Fatalf("expected no events, got %d", got)
	}
	if ls[0] != "BEGIN:VCALENDAR" || ls[len(ls)-1] != "END:VCALENDAR" {
		t.Error("empty calendar should still have wrapper")
	}
}
