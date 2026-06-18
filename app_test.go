package main

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// testClock is a fixed "now" (local noon, well clear of any midnight edge) so
// that date-relative tests are deterministic: both the app and the day() helper
// derive every date from this same instant.
var testClock = time.Date(2026, 6, 12, 12, 0, 0, 0, time.Local)

// newTestApp returns an App backed by a SQLite store in a temp directory,
// bypassing the real user config dir, with a frozen clock.
func newTestApp(t *testing.T) *App {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return &App{
		store: store,
		now:   func() time.Time { return testClock },
	}
}

func sessionDates(t *Topic) []string {
	out := make([]string, len(t.Sessions))
	for i, s := range t.Sessions {
		out[i] = s.Date
	}
	return out
}

func TestAddSpacedSessionsReplaceAndMerge(t *testing.T) {
	a := newTestApp(t)
	topics, err := a.AddTopic("Maths", "")
	if err != nil {
		t.Fatal(err)
	}
	id := topics[0].ID

	// Seed one manual session and mark it done.
	topics, err = a.AddSession(id, "2026-06-01")
	if err != nil {
		t.Fatal(err)
	}
	doneID := topics[0].Sessions[0].ID
	if _, err := a.ToggleSession(id, doneID); err != nil {
		t.Fatal(err)
	}

	// Merge: existing date kept (same ID, still done), new dates added around it.
	topics, err = a.AddSpacedSessions(id, "2026-06-01", []int{0, 2}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := sessionDates(topics[0])
	want := []string{"2026-06-01", "2026-06-03"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge dates = %v, want %v", got, want)
	}
	if s := topics[0].Sessions[0]; s.ID != doneID || !s.Done {
		t.Fatalf("merge should keep the existing session untouched, got id=%s done=%v", s.ID, s.Done)
	}

	// Replace: everything cleared, only the new schedule remains.
	topics, err = a.AddSpacedSessions(id, "2026-06-10", []int{0, 1}, true)
	if err != nil {
		t.Fatal(err)
	}
	got = sessionDates(topics[0])
	want = []string{"2026-06-10", "2026-06-11"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replace dates = %v, want %v", got, want)
	}
	for _, s := range topics[0].Sessions {
		if s.Done {
			t.Fatalf("replace should reset done state, got done session on %s", s.Date)
		}
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	a := newTestApp(t)
	topics, err := a.AddTopic("Physics", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddSession(topics[0].ID, "2026-06-05"); err != nil {
		t.Fatal(err)
	}

	got, err := a.GetTopics()
	if err != nil {
		t.Fatal(err)
	}
	got[0].Name = "mutated"
	got[0].Sessions[0].Done = true
	got[0].Tags = append(got[0].Tags, "sneaky")

	fresh, err := a.GetTopics()
	if err != nil {
		t.Fatal(err)
	}
	if fresh[0].Name != "Physics" || fresh[0].Sessions[0].Done || len(fresh[0].Tags) != 0 {
		t.Fatalf("mutating a returned snapshot leaked into the store: %+v", fresh[0])
	}
}

// day returns the frozen testClock + n days as a YYYY-MM-DD string, matching
// the app's local-date convention. Anchoring to testClock (not time.Now) keeps
// expectations aligned with the app's injected clock across midnight.
func day(n int) string {
	return testClock.AddDate(0, 0, n).Format(dateLayout)
}

func TestToggleSessionStampsCompletedAt(t *testing.T) {
	a := newTestApp(t)
	topics, _ := a.AddTopic("Biology", "")
	topics, err := a.AddSession(topics[0].ID, day(0))
	if err != nil {
		t.Fatal(err)
	}
	id, sid := topics[0].ID, topics[0].Sessions[0].ID

	topics, err = a.ToggleSession(id, sid)
	if err != nil {
		t.Fatal(err)
	}
	if s := topics[0].Sessions[0]; !s.Done || s.CompletedAt == nil {
		t.Fatalf("toggle on: done=%v completedAt=%v", s.Done, s.CompletedAt)
	}
	topics, err = a.ToggleSession(id, sid)
	if err != nil {
		t.Fatal(err)
	}
	if s := topics[0].Sessions[0]; s.Done || s.CompletedAt != nil {
		t.Fatalf("toggle off should clear completedAt, got done=%v completedAt=%v", s.Done, s.CompletedAt)
	}
}

func TestRescheduleSession(t *testing.T) {
	a := newTestApp(t)
	topics, _ := a.AddTopic("History", "")
	id := topics[0].ID
	a.AddSession(id, day(-3))
	topics, _ = a.AddSession(id, day(0))
	overdueID := topics[0].Sessions[0].ID

	// Moving onto an occupied date drops the moved session.
	topics, err := a.RescheduleSession(id, overdueID, day(0))
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionDates(topics[0]); !reflect.DeepEqual(got, []string{day(0)}) {
		t.Fatalf("move onto occupied date = %v, want just %v", got, day(0))
	}

	// Moving to a free date just changes the date.
	sid := topics[0].Sessions[0].ID
	topics, err = a.RescheduleSession(id, sid, day(2))
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionDates(topics[0]); !reflect.DeepEqual(got, []string{day(2)}) {
		t.Fatalf("move to free date = %v, want %v", got, day(2))
	}
}

// countByDate returns how many sessions a topic has on date, split done/pending.
func countByDate(tp *Topic, date string) (done, pending int) {
	for _, s := range tp.Sessions {
		if s.Date != date {
			continue
		}
		if s.Done {
			done++
		} else {
			pending++
		}
	}
	return
}

func TestRescheduleSessionCoexistsWithDone(t *testing.T) {
	a := newTestApp(t)
	topics, _ := a.AddTopic("History", "")
	id := topics[0].ID
	a.AddSession(id, day(-2)) // overdue, pending
	topics, _ = a.AddSession(id, day(0))
	overdueID := topics[0].Sessions[0].ID
	todayID := topics[0].Sessions[1].ID
	a.ToggleSession(id, todayID) // mark today's session done

	// Moving the overdue review onto today (which has only a DONE session)
	// must NOT drop it — it coexists with the completed one.
	topics, err := a.RescheduleSession(id, overdueID, day(0))
	if err != nil {
		t.Fatal(err)
	}
	done, pending := countByDate(topics[0], day(0))
	if done != 1 || pending != 1 {
		t.Fatalf("day(0) done=%d pending=%d, want 1 done + 1 pending (coexist)", done, pending)
	}
}

func TestRescheduleOverdueSessions(t *testing.T) {
	a := newTestApp(t)

	topics, _ := a.AddTopic("Catching up", "")
	lone := topics[0].ID
	a.AddSession(lone, day(-2)) // only overdue → moves to today

	topics, _ = a.AddTopic("Covered", "")
	covered := topics[1].ID
	a.AddSession(covered, day(-3)) // overdue but today exists → dropped
	a.AddSession(covered, day(0))

	topics, _ = a.AddTopic("Shelved", "")
	shelved := topics[2].ID
	a.AddSession(shelved, day(-5))
	a.SetTopicArchived(shelved, true) // archived → untouched

	// Studied-today topic: an overdue pending session plus a DONE session today.
	// The done one doesn't cover the overdue review, so it still moves to today.
	topics, _ = a.AddTopic("Studied today", "")
	studied := topics[3].ID
	a.AddSession(studied, day(-1))
	topics, _ = a.AddSession(studied, day(0))
	a.ToggleSession(studied, topics[3].Sessions[1].ID) // day(0) done

	topics, err := a.RescheduleOverdueSessions()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*Topic{}
	for _, tp := range topics {
		byID[tp.ID] = tp
	}
	if got := sessionDates(byID[lone]); !reflect.DeepEqual(got, []string{day(0)}) {
		t.Fatalf("lone overdue = %v, want moved to today", got)
	}
	if got := sessionDates(byID[covered]); !reflect.DeepEqual(got, []string{day(0)}) {
		t.Fatalf("covered topic = %v, want surplus overdue dropped", got)
	}
	if got := sessionDates(byID[shelved]); !reflect.DeepEqual(got, []string{day(-5)}) {
		t.Fatalf("archived topic = %v, want untouched", got)
	}
	if done, pending := countByDate(byID[studied], day(0)); done != 1 || pending != 1 {
		t.Fatalf("studied-today topic: day(0) done=%d pending=%d, want 1 done + 1 pending", done, pending)
	}
}

func TestGradeSession(t *testing.T) {
	t.Run("good re-anchors remaining gaps to today", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		a.AddSession(id, day(-4))
		a.AddSession(id, day(-1))
		topics, _ = a.AddSession(id, day(2))
		gradedID := topics[0].Sessions[0].ID // the day(-4) session

		topics, err := a.GradeSession(id, gradedID, "good")
		if err != nil {
			t.Fatal(err)
		}
		// Gaps from day(-4) were 3 and 6 days; ×1.0 re-anchored to today.
		want := []string{day(-4), day(3), day(6)}
		if got := sessionDates(topics[0]); !reflect.DeepEqual(got, want) {
			t.Fatalf("dates = %v, want %v", got, want)
		}
		if s := topics[0].Sessions[0]; !s.Done || s.CompletedAt == nil {
			t.Fatalf("graded session should be done with completedAt set")
		}
	})

	t.Run("again forces tomorrow and compresses", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		a.AddSession(id, day(0))
		a.AddSession(id, day(2))
		topics, _ = a.AddSession(id, day(6))
		gradedID := topics[0].Sessions[0].ID

		topics, err := a.GradeSession(id, gradedID, "again")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{day(0), day(1), day(3)}
		if got := sessionDates(topics[0]); !reflect.DeepEqual(got, want) {
			t.Fatalf("dates = %v, want %v", got, want)
		}
	})

	t.Run("again with no future sessions schedules tomorrow", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		topics, _ = a.AddSession(id, day(0))
		gradedID := topics[0].Sessions[0].ID

		topics, err := a.GradeSession(id, gradedID, "again")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{day(0), day(1)}
		if got := sessionDates(topics[0]); !reflect.DeepEqual(got, want) {
			t.Fatalf("dates = %v, want %v", got, want)
		}
	})

	t.Run("again schedules tomorrow even if tomorrow is already done", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		a.AddSession(id, day(0))
		topics, _ = a.AddSession(id, day(1))
		a.ToggleSession(id, topics[0].Sessions[1].ID) // day(1) reviewed early, done
		gradedID := topics[0].Sessions[0].ID

		topics, err := a.GradeSession(id, gradedID, "again")
		if err != nil {
			t.Fatal(err)
		}
		// A done session on tomorrow must not swallow the forced re-review:
		// a new pending session is scheduled for tomorrow, coexisting with it.
		if done, pending := countByDate(topics[0], day(1)); done != 1 || pending != 1 {
			t.Fatalf("day(1) done=%d pending=%d, want 1 done + 1 pending", done, pending)
		}
	})

	t.Run("again forces tomorrow past a done session, not after it", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		a.AddSession(id, day(0))
		a.AddSession(id, day(1))
		topics, _ = a.AddSession(id, day(5))
		a.ToggleSession(id, topics[0].Sessions[1].ID) // day(1) done
		gradedID := topics[0].Sessions[0].ID

		topics, err := a.GradeSession(id, gradedID, "again")
		if err != nil {
			t.Fatal(err)
		}
		// The remaining future review (was day(5)) re-anchors to tomorrow; the
		// done day(1) no longer pushes it out to day(2).
		if done, pending := countByDate(topics[0], day(1)); done != 1 || pending != 1 {
			t.Fatalf("day(1) done=%d pending=%d, want first review on tomorrow alongside the done one", done, pending)
		}
		if _, pending := countByDate(topics[0], day(2)); pending != 0 {
			t.Fatalf("day(2) should have no review; the forced tomorrow was not bumped")
		}
	})

	t.Run("compressed dates stay strictly increasing", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		a.AddSession(id, day(0))
		a.AddSession(id, day(1))
		topics, _ = a.AddSession(id, day(2))
		gradedID := topics[0].Sessions[0].ID

		// hard: round(1×0.7)=1 and round(2×0.7)=1 would collide; the second
		// bumps to keep increasing.
		topics, err := a.GradeSession(id, gradedID, "hard")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{day(0), day(1), day(2)}
		if got := sessionDates(topics[0]); !reflect.DeepEqual(got, want) {
			t.Fatalf("dates = %v, want %v", got, want)
		}
	})

	t.Run("rejects unknown grades and done sessions", func(t *testing.T) {
		a := newTestApp(t)
		topics, _ := a.AddTopic("Maths", "")
		id := topics[0].ID
		topics, _ = a.AddSession(id, day(0))
		sid := topics[0].Sessions[0].ID

		if _, err := a.GradeSession(id, sid, "amazing"); err == nil {
			t.Fatal("expected error for unknown grade")
		}
		a.ToggleSession(id, sid)
		if _, err := a.GradeSession(id, sid, "good"); err == nil {
			t.Fatal("expected error for already-done session")
		}
	})
}

func TestDeleteSessionNotFound(t *testing.T) {
	a := newTestApp(t)
	topics, err := a.AddTopic("Chemistry", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeleteSession(topics[0].ID, "nope"); err == nil {
		t.Fatal("expected an error for an unknown session id")
	}
}

func TestSpacedDates(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := spacedDates(start, []int{7, 0, 0, 3, -2})
	want := []string{"2026-06-01", "2026-06-04", "2026-06-08"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spacedDates = %v, want %v", got, want)
	}
}

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags([]string{" Go ", "go", "", "GO", "rust"})
	want := []string{"Go", "rust"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTags = %v, want %v", got, want)
	}

	long := make([]string, 20)
	for i := range long {
		long[i] = string(rune('a'+i)) + "-tag"
	}
	if got := normalizeTags(long); len(got) != 12 {
		t.Fatalf("tag count cap = %d, want 12", len(got))
	}
	if got := normalizeTags([]string{"abcdefghijklmnopqrstuvwxyz0123456789"}); len([]rune(got[0])) != 30 {
		t.Fatalf("tag length cap = %d, want 30", len([]rune(got[0])))
	}
}

func TestPickColor(t *testing.T) {
	// Sequential adds (no deletes) reproduce the plain round-robin cycle.
	var topics []*Topic
	for i, want := range []string{"blue", "violet", "emerald"} {
		if got := pickColor(topics); got != want {
			t.Fatalf("add %d: pickColor = %q, want %q", i, got, want)
		}
		topics = append(topics, &Topic{Color: pickColor(topics)})
	}

	// After deleting the only "blue" topic, the next add reuses blue (the now
	// least-used token) rather than blindly continuing the cycle.
	topics = []*Topic{{Color: "violet"}, {Color: "emerald"}}
	if got := pickColor(topics); got != "blue" {
		t.Fatalf("pickColor after delete = %q, want blue (least used)", got)
	}

	// Reset ("") colours don't count against any palette token.
	topics = []*Topic{{Color: ""}, {Color: ""}}
	if got := pickColor(topics); got != "blue" {
		t.Fatalf("pickColor with reset colours = %q, want blue", got)
	}
}

func TestNormalizeOrder(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	topics := []*Topic{
		{ID: "b", Order: 5, CreatedAt: base.Add(2 * time.Hour)},
		{ID: "a", Order: 0, CreatedAt: base.Add(time.Hour)},
		{ID: "c", Order: 0, CreatedAt: base},
	}
	normalizeOrder(topics)
	gotIDs := []string{topics[0].ID, topics[1].ID, topics[2].ID}
	if want := []string{"c", "a", "b"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("order = %v, want %v", gotIDs, want)
	}
	for i, tp := range topics {
		if tp.Order != i {
			t.Fatalf("topic %s has Order %d, want %d", tp.ID, tp.Order, i)
		}
	}
}

func TestRecordFocusSession(t *testing.T) {
	a := newTestApp(t)
	topics, err := a.AddTopic("Maths", "")
	if err != nil {
		t.Fatal(err)
	}
	topicID := topics[0].ID

	// A focus block against a topic is logged and stamped with the clock.
	focus, err := a.RecordFocusSession(topicID, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(focus) != 1 {
		t.Fatalf("focus count = %d, want 1", len(focus))
	}
	if focus[0].TopicID != topicID || focus[0].DurationSec != 1500 {
		t.Fatalf("focus = %+v, want topic %s / 1500s", focus[0], topicID)
	}
	if !focus[0].CompletedAt.Equal(testClock) {
		t.Fatalf("completedAt = %v, want %v", focus[0].CompletedAt, testClock)
	}

	// General focus ("" topic) is allowed.
	if focus, err = a.RecordFocusSession("", 600); err != nil {
		t.Fatal(err)
	}
	if len(focus) != 2 {
		t.Fatalf("focus count = %d, want 2", len(focus))
	}

	// Non-positive duration and unknown topic are rejected.
	if _, err := a.RecordFocusSession(topicID, 0); err == nil {
		t.Fatal("expected error for zero duration")
	}
	if _, err := a.RecordFocusSession("nope", 60); err == nil {
		t.Fatal("expected error for unknown topic")
	}
}

func TestFocusSurvivesTopicMutationAndReload(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	store, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{store: store, now: func() time.Time { return testClock }}

	topics, _ := a.AddTopic("Maths", "")
	topicID := topics[0].ID
	if _, err := a.RecordFocusSession(topicID, 1500); err != nil {
		t.Fatal(err)
	}

	// A topic mutation rewrites the whole topic graph via save(); the focus log
	// must not be wiped by that. Deleting the topic leaves the focus row intact
	// (its topic_id simply dangles).
	if _, err := a.DeleteTopic(topicID); err != nil {
		t.Fatal(err)
	}
	focus, err := a.GetFocusSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(focus) != 1 || focus[0].TopicID != topicID {
		t.Fatalf("focus after topic delete = %+v, want 1 record keeping its topic id", focus)
	}
	store.Close()

	// And it persists across a reopen.
	reopened, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.focus) != 1 || reopened.focus[0].DurationSec != 1500 {
		t.Fatalf("focus after reload = %+v, want 1 record of 1500s", reopened.focus)
	}
}
