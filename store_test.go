package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// newTestStore opens an empty SQLite store in a fresh temp directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	s, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)
	completed := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	s.topics = []*Topic{{
		ID:          "t1",
		Name:        "Maths",
		Description: "calc",
		Color:       "violet",
		Tags:        []string{"charlie", "alpha", "bravo"}, // deliberately unsorted
		Archived:    true,
		Adaptive:    true,
		Order:       0,
		CreatedAt:   created,
		Sessions: []*Session{
			{ID: "s1", Date: "2026-06-01", Done: false},
			{ID: "s2", Date: "2026-06-02", Done: true, CompletedAt: &completed},
		},
	}}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Reopen from disk — no import (db already populated), pure load path.
	reopened, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	if len(reopened.topics) != 1 {
		t.Fatalf("topic count = %d, want 1", len(reopened.topics))
	}
	got := reopened.topics[0]
	if got.Name != "Maths" || got.Description != "calc" || got.Color != "violet" {
		t.Fatalf("scalar fields not round-tripped: %+v", got)
	}
	if !got.Archived || !got.Adaptive {
		t.Fatalf("archived=%v adaptive=%v, want both true", got.Archived, got.Adaptive)
	}
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("createdAt = %v, want %v", got.CreatedAt, created)
	}
	// Tag order must be preserved exactly (not re-sorted).
	if want := []string{"charlie", "alpha", "bravo"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags = %v, want %v", got.Tags, want)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(got.Sessions))
	}
	bySID := map[string]*Session{}
	for _, sess := range got.Sessions {
		bySID[sess.ID] = sess
	}
	if s1 := bySID["s1"]; s1 == nil || s1.Done || s1.CompletedAt != nil {
		t.Fatalf("pending session not round-tripped: %+v", s1)
	}
	if s2 := bySID["s2"]; s2 == nil || !s2.Done || s2.CompletedAt == nil || !s2.CompletedAt.Equal(completed) {
		t.Fatalf("done session not round-tripped: %+v", s2)
	}
}

func TestFirstRunImportsLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "data.json")
	legacy := `[{"id":"t1","name":"Imported","description":"","color":"blue",` +
		`"tags":["go"],"archived":false,"adaptive":false,"order":0,` +
		`"createdAt":"2026-06-01T09:30:00Z","sessions":[` +
		`{"id":"s1","date":"2026-06-01","done":false}]}]`
	if err := os.WriteFile(jsonPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := openStore(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if len(s.topics) != 1 || s.topics[0].Name != "Imported" {
		t.Fatalf("legacy import failed: %+v", s.topics)
	}
	if len(s.topics[0].Sessions) != 1 || s.topics[0].Sessions[0].Date != "2026-06-01" {
		t.Fatalf("legacy sessions not imported: %+v", s.topics[0].Sessions)
	}
	// The JSON backup must be left in place.
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("data.json should be kept as backup: %v", err)
	}
}

func TestImportSkippedWhenDBPopulated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	// Seed the database with one topic.
	s, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s.topics = []*Topic{{ID: "keep", Name: "Existing", CreatedAt: time.Now(), Tags: []string{}, Sessions: []*Session{}}}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Now drop a data.json beside it; reopening must NOT import over real data.
	legacy := `[{"id":"x","name":"Should not import","tags":[],"sessions":[],"createdAt":"2026-06-01T09:30:00Z"}]`
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.topics) != 1 || reopened.topics[0].Name != "Existing" {
		t.Fatalf("import should be skipped when db is populated, got %+v", reopened.topics)
	}
}

func TestSaveCascadesDeletes(t *testing.T) {
	s := newTestStore(t)
	s.topics = []*Topic{{
		ID: "t1", Name: "Doomed", CreatedAt: time.Now(),
		Tags:     []string{"a", "b"},
		Sessions: []*Session{{ID: "s1", Date: "2026-06-01"}},
	}}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	// Remove the topic and persist; the DELETE FROM topics in save() must
	// cascade to its sessions and tags (foreign_keys=1).
	s.topics = []*Topic{}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	for _, tbl := range []string{"topics", "sessions", "topic_tags"} {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d rows after cascade delete", tbl, n)
		}
	}
}
