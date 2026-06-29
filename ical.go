package main

import (
	"sort"
	"strings"
	"time"
)

// iCalProdID identifies this app as the generator of the calendar, per RFC 5545.
const iCalProdID = "-//study-planner//Study Planner//EN"

// icalDateLayout formats a date as an RFC 5545 DATE value (no time component),
// used for all-day events. icalStampLayout is the UTC DATE-TIME used for DTSTAMP.
const (
	icalDateLayout  = "20060102"
	icalStampLayout = "20060102T150405Z"
)

// buildICS renders the outstanding study schedule as an RFC 5545 calendar.
// Every not-done session of every non-archived task becomes one all-day VEVENT,
// so importing the file drops the whole pending schedule (overdue included, it
// being a snapshot) into Google, Apple or Outlook calendars. Done sessions are
// history and archived tasks are hidden, so neither is exported. now stamps each
// event's DTSTAMP; passing it in keeps the output deterministic for tests.
//
// Events are sorted by date then UID so the output is stable regardless of task
// order. Each UID is the session id, so re-importing an updated file updates the
// existing events in calendars that key on UID rather than duplicating them.
func buildICS(tasks []*Task, now time.Time) string {
	type event struct {
		uid, date, summary, description string
	}
	var events []event
	for _, t := range tasks {
		if t.Archived {
			continue
		}
		for _, s := range t.Sessions {
			if s.Done {
				continue
			}
			if _, err := time.Parse(dateLayout, s.Date); err != nil {
				continue // skip malformed dates rather than emit a broken event
			}
			events = append(events, event{
				uid:         s.ID,
				date:        s.Date,
				summary:     "Study: " + t.Name,
				description: t.Description,
			})
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].date != events[j].date {
			return events[i].date < events[j].date
		}
		return events[i].uid < events[j].uid
	})

	stamp := now.UTC().Format(icalStampLayout)
	var b strings.Builder
	writeLine := func(s string) {
		b.WriteString(foldICSLine(s))
		b.WriteString("\r\n")
	}

	writeLine("BEGIN:VCALENDAR")
	writeLine("VERSION:2.0")
	writeLine("PRODID:" + iCalProdID)
	writeLine("CALSCALE:GREGORIAN")
	writeLine("METHOD:PUBLISH")
	writeLine("X-WR-CALNAME:Study Planner")
	for _, e := range events {
		start, _ := time.Parse(dateLayout, e.date)
		end := start.AddDate(0, 0, 1) // DTEND is exclusive; all-day spans one day
		writeLine("BEGIN:VEVENT")
		writeLine("UID:" + e.uid + "@study-planner")
		writeLine("DTSTAMP:" + stamp)
		writeLine("DTSTART;VALUE=DATE:" + start.Format(icalDateLayout))
		writeLine("DTEND;VALUE=DATE:" + end.Format(icalDateLayout))
		writeLine("SUMMARY:" + escapeICSText(e.summary))
		if e.description != "" {
			writeLine("DESCRIPTION:" + escapeICSText(e.description))
		}
		writeLine("END:VEVENT")
	}
	writeLine("END:VCALENDAR")
	return b.String()
}

// escapeICSText escapes a value for an RFC 5545 TEXT field: backslash, semicolon
// and comma are backslash-escaped, and CR/LF become the literal "\n" sequence.
// Colons are not special in TEXT values and are left alone.
func escapeICSText(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
	)
	return r.Replace(s)
}

// foldICSLine folds a content line to the RFC 5545 limit of 75 octets per line,
// continuing with CRLF + a single leading space. Folding happens on rune
// boundaries so multi-byte UTF-8 characters are never split.
func foldICSLine(line string) string {
	const limit = 75
	if len(line) <= limit {
		return line
	}
	var b strings.Builder
	count := 0    // octets emitted on the current physical line
	first := true // the first physical line has no leading space
	for _, r := range line {
		n := len(string(r))
		// Continuation lines start with a space, leaving 74 octets for content.
		max := limit
		if !first {
			max = limit - 1
		}
		if count+n > max {
			b.WriteString("\r\n ")
			count = 0
			first = false
		}
		b.WriteRune(r)
		count += n
	}
	return b.String()
}
