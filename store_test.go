package main

import (
	"database/sql"
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
	s.subjects = []*Subject{{ID: "sub1", Name: "Science", Color: "emerald", Order: 0, CreatedAt: created}}
	s.tasks = []*Task{{
		ID:          "t1",
		Name:        "Maths",
		Description: "calc",
		Color:       "violet",
		SubjectID:   "sub1",
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

	if len(reopened.subjects) != 1 || reopened.subjects[0].Name != "Science" || reopened.subjects[0].Color != "emerald" {
		t.Fatalf("subject not round-tripped: %+v", reopened.subjects)
	}
	if len(reopened.tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(reopened.tasks))
	}
	got := reopened.tasks[0]
	if got.Name != "Maths" || got.Description != "calc" || got.Color != "violet" || got.SubjectID != "sub1" {
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

	if len(s.tasks) != 1 || s.tasks[0].Name != "Imported" {
		t.Fatalf("legacy import failed: %+v", s.tasks)
	}
	// Legacy data predates subjects, so imported tasks are ungrouped.
	if s.tasks[0].SubjectID != "" {
		t.Fatalf("imported task subjectID = %q, want empty", s.tasks[0].SubjectID)
	}
	if len(s.tasks[0].Sessions) != 1 || s.tasks[0].Sessions[0].Date != "2026-06-01" {
		t.Fatalf("legacy sessions not imported: %+v", s.tasks[0].Sessions)
	}
	// The JSON backup must be left in place.
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("data.json should be kept as backup: %v", err)
	}
}

func TestImportSkippedWhenDBPopulated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	// Seed the database with one task.
	s, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s.tasks = []*Task{{ID: "keep", Name: "Existing", CreatedAt: time.Now(), Tags: []string{}, Sessions: []*Session{}}}
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
	if len(reopened.tasks) != 1 || reopened.tasks[0].Name != "Existing" {
		t.Fatalf("import should be skipped when db is populated, got %+v", reopened.tasks)
	}
}

func TestSaveCascadesDeletes(t *testing.T) {
	s := newTestStore(t)
	s.subjects = []*Subject{{ID: "sub1", Name: "Doomed subject", CreatedAt: time.Now()}}
	s.tasks = []*Task{{
		ID: "t1", Name: "Doomed", CreatedAt: time.Now(), SubjectID: "sub1",
		Tags:     []string{"a", "b"},
		Sessions: []*Session{{ID: "s1", Date: "2026-06-01"}},
	}}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	// Remove the task and subject and persist; the DELETE FROM tasks in save()
	// must cascade to its sessions and tags (foreign_keys=1), and subjects are
	// rewritten too.
	s.subjects = []*Subject{}
	s.tasks = []*Task{}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	for _, tbl := range []string{"subjects", "tasks", "sessions", "task_tags"} {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d rows after cascade delete", tbl, n)
		}
	}
}

// schemaV2 is the pre-v3, topic-centric layout, captured here so the migration
// test can build a realistic legacy database to step up.
const schemaV2 = `
CREATE TABLE topics (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  color       TEXT NOT NULL DEFAULT '',
  archived    INTEGER NOT NULL DEFAULT 0,
  adaptive    INTEGER NOT NULL DEFAULT 0,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);
CREATE TABLE sessions (
  id           TEXT PRIMARY KEY,
  topic_id     TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  done         INTEGER NOT NULL DEFAULT 0,
  completed_at TEXT
);
CREATE TABLE topic_tags (
  topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  tag      TEXT NOT NULL,
  position INTEGER NOT NULL,
  PRIMARY KEY (topic_id, tag)
);
CREATE INDEX idx_sessions_topic ON sessions(topic_id);
CREATE INDEX idx_sessions_date  ON sessions(date);
CREATE TABLE focus_sessions (
  id           TEXT PRIMARY KEY,
  topic_id     TEXT NOT NULL DEFAULT '',
  duration_sec INTEGER NOT NULL,
  completed_at TEXT NOT NULL
);
CREATE INDEX idx_focus_completed ON focus_sessions(completed_at);
PRAGMA user_version = 2;
`

// TestMigrateV2toV3 builds a v2 database with real data and asserts openStore
// migrates it in place: topics become tasks (ungrouped), tags/sessions and the
// focus log all survive, and the version is bumped.
func TestMigrateV2toV3(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	// Hand-build a v2 database, bypassing openStore's migration.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV2); err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`INSERT INTO topics (id, name, description, color, archived, adaptive, sort_order, created_at)
		 VALUES ('t1', 'Maths', 'calc', 'violet', 1, 1, 0, '2026-06-01T09:30:00Z')`,
		`INSERT INTO sessions (id, topic_id, date, done, completed_at)
		 VALUES ('s1', 't1', '2026-06-01', 0, NULL)`,
		`INSERT INTO topic_tags (topic_id, tag, position) VALUES ('t1', 'algebra', 0)`,
		`INSERT INTO focus_sessions (id, topic_id, duration_sec, completed_at)
		 VALUES ('f1', 't1', 1500, '2026-06-01T10:00:00Z')`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	// Reopen through the normal path: migrate() must step v2 → v3.
	s, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore on a v2 database failed: %v", err)
	}
	defer s.Close()

	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}

	if len(s.tasks) != 1 {
		t.Fatalf("task count after migration = %d, want 1", len(s.tasks))
	}
	got := s.tasks[0]
	if got.Name != "Maths" || got.Color != "violet" || !got.Archived || !got.Adaptive {
		t.Fatalf("task fields lost in migration: %+v", got)
	}
	if got.SubjectID != "" {
		t.Fatalf("migrated task subjectID = %q, want empty (ungrouped)", got.SubjectID)
	}
	if want := []string{"algebra"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags after migration = %v, want %v", got.Tags, want)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Date != "2026-06-01" {
		t.Fatalf("sessions after migration = %+v", got.Sessions)
	}
	if len(s.focus) != 1 || s.focus[0].TaskID != "t1" || s.focus[0].DurationSec != 1500 {
		t.Fatalf("focus log after migration = %+v", s.focus)
	}
	// The new subjects table exists and is empty.
	if len(s.subjects) != 0 {
		t.Fatalf("subjects after migration = %+v, want none", s.subjects)
	}
}

// TestSettingsPersistence covers the v4 key/value settings: a fresh store starts
// at the default goal, and a set value survives a reopen.
func TestSettingsPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	s, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if s.settings.DailyGoalMinutes != defaultDailyGoalMinutes {
		t.Fatalf("fresh daily goal = %d, want default %d", s.settings.DailyGoalMinutes, defaultDailyGoalMinutes)
	}
	if err := s.setDailyGoalMinutes(90); err != nil {
		t.Fatal(err)
	}
	s.Close()

	reopened, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.settings.DailyGoalMinutes != 90 {
		t.Fatalf("reloaded daily goal = %d, want 90", reopened.settings.DailyGoalMinutes)
	}
}

// TestMigrateV3toV4 builds a v3 database (a v2 stepped up to v3, so it has no
// settings table) and asserts openStore adds the table, bumps the version, and
// applies the default goal for the missing row.
func TestMigrateV3toV4(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV2); err != nil {
		t.Fatal(err)
	}
	// Step to v3 only, leaving the database one version behind current.
	if err := execTx(db, migrateV2toV3); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 3 {
		t.Fatalf("hand-built database is v%d, want v3", v)
	}
	db.Close()

	s, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore on a v3 database failed: %v", err)
	}
	defer s.Close()

	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	if s.settings.DailyGoalMinutes != defaultDailyGoalMinutes {
		t.Fatalf("migrated daily goal = %d, want default %d", s.settings.DailyGoalMinutes, defaultDailyGoalMinutes)
	}
	// The settings row can be written post-migration.
	if err := s.setDailyGoalMinutes(45); err != nil {
		t.Fatalf("setDailyGoalMinutes after migration: %v", err)
	}
}
