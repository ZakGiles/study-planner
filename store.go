package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store holds all tasks and subjects in memory and persists them to a SQLite
// database. The in-memory slices stay the authoritative working copy that app.go
// mutates; save() rewrites the whole graph to SQLite in one transaction.
type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	jsonPath string // legacy data.json, used for one-time import and kept as backup
	tasks    []*Task
	subjects []*Subject
	// focus is the completed-focus-block log. It lives outside the task graph
	// rewritten by save(): records are appended individually and never deleted
	// by task mutations (see the focus_sessions schema comment).
	focus []*FocusSession
	// settings holds user preferences persisted in the key/value settings table,
	// written with single-row upserts independent of save().
	settings Settings
}

// schemaVersion is the current PRAGMA user_version. v3 renamed topics→tasks
// (and the topic_id columns→task_id) and introduced subjects; v4 added the
// key/value settings table.
const schemaVersion = 4

// schema is the current database layout, applied to fresh databases. Existing
// databases are stepped up by migrate(). There is deliberately no
// UNIQUE(task_id, date) on sessions: a done (historical) session and a pending
// session may share a day. The "at most one pending session per (task, date)"
// invariant is enforced in app.go, not the database.
const schema = `
CREATE TABLE IF NOT EXISTS subjects (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  color       TEXT NOT NULL DEFAULT '',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  color       TEXT NOT NULL DEFAULT '',
  subject_id  TEXT NOT NULL DEFAULT '',
  archived    INTEGER NOT NULL DEFAULT 0,
  adaptive    INTEGER NOT NULL DEFAULT 0,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id           TEXT PRIMARY KEY,
  task_id      TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  done         INTEGER NOT NULL DEFAULT 0,
  completed_at TEXT
);
CREATE TABLE IF NOT EXISTS task_tags (
  task_id  TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  tag      TEXT NOT NULL,
  position INTEGER NOT NULL,
  PRIMARY KEY (task_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_sessions_task ON sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_sessions_date ON sessions(date);
-- subject_id on tasks is a plain string ("" = ungrouped), NOT a foreign key:
-- save() rewrites the whole tasks table on every mutation, so a cascading FK
-- onto subjects would be fragile; deleting a subject ungroups its tasks in
-- app.go instead.
-- focus_sessions deliberately has NO foreign key to tasks: save() rewrites the
-- whole tasks table on every mutation, so a cascading FK would wipe focus
-- history. task_id is a plain string ("" = general focus); a deleted task just
-- leaves a dangling id the frontend renders as general.
CREATE TABLE IF NOT EXISTS focus_sessions (
  id           TEXT PRIMARY KEY,
  task_id      TEXT NOT NULL DEFAULT '',
  duration_sec INTEGER NOT NULL,
  completed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_focus_completed ON focus_sessions(completed_at);
-- settings is a small key/value bag for user preferences that travel with the
-- data (the daily review goal), plus internal flags (the one-shot legacy-import
-- latch). Like focus_sessions it is written with single-row upserts, independent
-- of the whole-graph save().
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
PRAGMA user_version = 4;
`

// migrateV3toV4 adds the settings table to an existing v3 database. Run inside a
// transaction by migrate().
const migrateV3toV4 = `
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
PRAGMA user_version = 4;
`

// migrateV2toV3 renames the v2 topic-centric layout to the v3 task/subject
// layout: tables topics→tasks and topic_tags→task_tags, the topic_id columns on
// sessions/task_tags/focus_sessions→task_id, plus a new subject_id column and
// the subjects table. SQLite rewrites child-table foreign-key references when a
// table is renamed, so sessions/task_tags keep pointing at tasks(id). Run inside
// a transaction by migrate().
const migrateV2toV3 = `
ALTER TABLE topics RENAME TO tasks;
ALTER TABLE tasks ADD COLUMN subject_id TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_tags RENAME TO task_tags;
ALTER TABLE task_tags RENAME COLUMN topic_id TO task_id;
ALTER TABLE sessions RENAME COLUMN topic_id TO task_id;
ALTER TABLE focus_sessions RENAME COLUMN topic_id TO task_id;
DROP INDEX IF EXISTS idx_sessions_topic;
CREATE INDEX IF NOT EXISTS idx_sessions_task ON sessions(task_id);
CREATE TABLE IF NOT EXISTS subjects (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  color       TEXT NOT NULL DEFAULT '',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);
PRAGMA user_version = 3;
`

// migrate brings the database up to schemaVersion. A brand-new, empty database
// (user_version 0, no tables) gets the full current schema; a v2 database is
// stepped to v3 in one transaction; an already-current database is left alone.
func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return fmt.Errorf("unsupported database schema version %d", v)
	}
	// Apply one migration step at a time, re-reading user_version after each, so a
	// database several versions behind is stepped all the way up in one startup.
	for v < schemaVersion {
		switch v {
		case 0:
			// Fresh database (or one created by this version): apply current schema.
			if _, err := db.Exec(schema); err != nil {
				return err
			}
		case 2:
			if err := execTx(db, migrateV2toV3); err != nil {
				return err
			}
		case 3:
			if err := execTx(db, migrateV3toV4); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported database schema version %d", v)
		}
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
			return err
		}
	}
	return nil
}

// execTx runs a multi-statement SQL script in a single transaction.
func execTx(db *sql.DB, script string) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if _, err = tx.Exec(script); err != nil {
		return err
	}
	return tx.Commit()
}

// NewStore creates a store backed by data.db inside the user's config directory
// (e.g. ~/Library/Application Support/study-planner on macOS), importing a legacy
// data.json on first run if one exists.
func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(dir, "study-planner")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, err
	}
	return openStore(filepath.Join(appDir, "data.db"))
}

// openStore opens (creating if needed) the SQLite database at dbPath, applies the
// schema, performs the one-time JSON import, and loads everything into memory. A
// legacy data.json is looked for next to the database. Factored out of NewStore
// so tests can point it at a temp directory.
func openStore(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		dbPath,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{
		db:       db,
		jsonPath: filepath.Join(filepath.Dir(dbPath), "data.json"),
		tasks:    []*Task{},
		subjects: []*Subject{},
		settings: Settings{DailyGoalMinutes: defaultDailyGoalMinutes},
	}
	if err := s.importLegacyJSON(); err != nil {
		// Import is best-effort: a corrupt backup must not block startup.
		log.Printf("study-planner: legacy data.json import skipped: %v", err)
	}
	if err := s.load(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.loadFocusSessions(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.loadSettings(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// legacyImportedKey is the settings latch marking the one-time data.json import
// as done (or deliberately skipped). Without it, the import would re-fire on any
// later startup where the tasks table happens to be empty — data.json is kept
// forever as a backup — wiping subjects (save() runs before load(), while the
// in-memory subject list is still empty) and resurrecting deleted tasks.
const legacyImportedKey = "legacy_imported"

// importLegacyJSON imports a legacy data.json into the database once, on a
// store's true first run, and leaves the JSON file in place as a backup. The
// settings latch makes it one-shot; a database holding any real data (tasks,
// subjects or focus history) is latched without importing so the backup can
// never overwrite live state. A fresh, empty store with no data.json stays
// unlatched, so dropping a backup in before first use still restores it.
func (s *Store) importLegacyJSON() error {
	if _, ok, err := s.getSetting(legacyImportedKey); err != nil {
		return err
	} else if ok {
		return nil // already imported (or skipped) on an earlier run
	}
	for _, table := range []string{"tasks", "subjects", "focus_sessions"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			// Real data exists (predating the latch): never import over it.
			return s.setSetting(legacyImportedKey, "1")
		}
	}
	tasks, err := readLegacyJSON(s.jsonPath)
	if err != nil {
		return err
	}
	if tasks == nil {
		return nil // no data.json present; leave unlatched for a later restore
	}
	if len(tasks) > 0 {
		s.tasks = tasks
		if err := s.save(); err != nil {
			return err
		}
	}
	return s.setSetting(legacyImportedKey, "1")
}

// readLegacyJSON reads and normalizes tasks from a legacy data.json. A missing
// file returns (nil, nil); an empty file returns an empty slice. Legacy data
// predates subjects, so imported tasks are ungrouped (SubjectID == "").
func readLegacyJSON(path string) ([]*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []*Task{}, nil
	}
	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []*Task{}
	}
	return tasks, nil
}

// load reads all subjects, tasks, sessions and tags from the database into memory.
func (s *Store) load() error {
	if err := s.loadSubjects(); err != nil {
		return err
	}

	rows, err := s.db.Query(
		`SELECT id, name, description, color, subject_id, archived, adaptive, sort_order, created_at
		 FROM tasks ORDER BY sort_order, created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tasks []*Task
	byID := make(map[string]*Task)
	for rows.Next() {
		t := &Task{Tags: []string{}, Sessions: []*Session{}}
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Color, &t.SubjectID,
			&t.Archived, &t.Adaptive, &t.Order, &createdAt); err != nil {
			return err
		}
		if t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return fmt.Errorf("task %s: bad created_at %q: %w", t.ID, createdAt, err)
		}
		tasks = append(tasks, t)
		byID[t.ID] = t
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if err := s.loadTags(byID); err != nil {
		return err
	}
	if err := s.loadSessions(byID); err != nil {
		return err
	}

	if tasks == nil {
		tasks = []*Task{}
	}
	normalizeOrder(tasks)
	s.tasks = tasks
	return nil
}

// loadSubjects reads all subjects into memory, ordered by sort_order.
func (s *Store) loadSubjects() error {
	rows, err := s.db.Query(
		`SELECT id, name, color, sort_order, created_at FROM subjects ORDER BY sort_order, created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()
	subjects := []*Subject{}
	for rows.Next() {
		sub := &Subject{}
		var createdAt string
		if err := rows.Scan(&sub.ID, &sub.Name, &sub.Color, &sub.Order, &createdAt); err != nil {
			return err
		}
		if sub.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return fmt.Errorf("subject %s: bad created_at %q: %w", sub.ID, createdAt, err)
		}
		subjects = append(subjects, sub)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	normalizeSubjectOrder(subjects)
	s.subjects = subjects
	return nil
}

// loadTags attaches tags (in stored order) to the tasks in byID.
func (s *Store) loadTags(byID map[string]*Task) error {
	rows, err := s.db.Query(`SELECT task_id, tag FROM task_tags ORDER BY task_id, position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var taskID, tag string
		if err := rows.Scan(&taskID, &tag); err != nil {
			return err
		}
		if t := byID[taskID]; t != nil {
			t.Tags = append(t.Tags, tag)
		}
	}
	return rows.Err()
}

// loadSessions attaches sessions to the tasks in byID. Ordering is irrelevant
// here because snapshot() re-sorts sessions by date before serving them.
func (s *Store) loadSessions(byID map[string]*Task) error {
	rows, err := s.db.Query(`SELECT id, task_id, date, done, completed_at FROM sessions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var taskID, completedAt string
		var nullCompleted sql.NullString
		sess := &Session{}
		if err := rows.Scan(&sess.ID, &taskID, &sess.Date, &sess.Done, &nullCompleted); err != nil {
			return err
		}
		if nullCompleted.Valid {
			completedAt = nullCompleted.String
			ts, err := time.Parse(time.RFC3339Nano, completedAt)
			if err != nil {
				return fmt.Errorf("session %s: bad completed_at %q: %w", sess.ID, completedAt, err)
			}
			sess.CompletedAt = &ts
		}
		if t := byID[taskID]; t != nil {
			t.Sessions = append(t.Sessions, sess)
		}
	}
	return rows.Err()
}

// save rewrites the entire in-memory graph (subjects + tasks) to the database in
// one transaction. Deleting all tasks cascades to sessions and tags
// (foreign_keys=1), so the re-insert below is a clean full replacement, matching
// the previous JSON save. Subjects are rewritten the same way.
func (s *Store) save() (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM subjects`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM tasks`); err != nil {
		return err
	}

	subjStmt, err := tx.Prepare(
		`INSERT INTO subjects (id, name, color, sort_order, created_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer subjStmt.Close()
	for _, sub := range s.subjects {
		if _, err = subjStmt.Exec(sub.ID, sub.Name, sub.Color, sub.Order,
			sub.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}

	taskStmt, err := tx.Prepare(
		`INSERT INTO tasks (id, name, description, color, subject_id, archived, adaptive, sort_order, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer taskStmt.Close()
	tagStmt, err := tx.Prepare(`INSERT INTO task_tags (task_id, tag, position) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer tagStmt.Close()
	sessStmt, err := tx.Prepare(
		`INSERT INTO sessions (id, task_id, date, done, completed_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer sessStmt.Close()

	for _, t := range s.tasks {
		if _, err = taskStmt.Exec(t.ID, t.Name, t.Description, t.Color, t.SubjectID,
			t.Archived, t.Adaptive, t.Order, t.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		for i, tag := range t.Tags {
			if _, err = tagStmt.Exec(t.ID, tag, i); err != nil {
				return err
			}
		}
		for _, sess := range t.Sessions {
			var completed any
			if sess.CompletedAt != nil {
				completed = sess.CompletedAt.Format(time.RFC3339Nano)
			}
			if _, err = sessStmt.Exec(sess.ID, t.ID, sess.Date, sess.Done, completed); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// loadFocusSessions reads the whole focus log into memory, newest last. Ordering
// here is loose; the snapshot served to the frontend sorts as needed.
func (s *Store) loadFocusSessions() error {
	rows, err := s.db.Query(
		`SELECT id, task_id, duration_sec, completed_at FROM focus_sessions ORDER BY completed_at`)
	if err != nil {
		return err
	}
	defer rows.Close()
	focus := []*FocusSession{}
	for rows.Next() {
		fs := &FocusSession{}
		var completedAt string
		if err := rows.Scan(&fs.ID, &fs.TaskID, &fs.DurationSec, &completedAt); err != nil {
			return err
		}
		if fs.CompletedAt, err = time.Parse(time.RFC3339Nano, completedAt); err != nil {
			return fmt.Errorf("focus session %s: bad completed_at %q: %w", fs.ID, completedAt, err)
		}
		focus = append(focus, fs)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.focus = focus
	return nil
}

// addFocusSession persists one completed focus block and appends it in memory.
// It inserts a single row rather than going through save(), keeping the focus
// log independent of the task-graph rewrite. The caller must hold the lock.
func (s *Store) addFocusSession(fs *FocusSession) error {
	if _, err := s.db.Exec(
		`INSERT INTO focus_sessions (id, task_id, duration_sec, completed_at) VALUES (?, ?, ?, ?)`,
		fs.ID, fs.TaskID, fs.DurationSec, fs.CompletedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	s.focus = append(s.focus, fs)
	return nil
}

// getSetting reads one settings row; ok is false when the key has never been
// written.
func (s *Store) getSetting(key string) (value string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return value, true, nil
}

// setSetting upserts one settings row. Like addFocusSession it writes a single
// row rather than going through save(), keeping settings independent of the
// task-graph rewrite.
func (s *Store) setSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// loadSettings reads persisted preferences into memory, leaving the in-memory
// defaults in place for any key the database doesn't have yet (e.g. a v3 database
// just migrated to v4 has an empty settings table).
func (s *Store) loadSettings() error {
	value, ok, err := s.getSetting("daily_goal_minutes")
	if err != nil || !ok {
		return err // on !ok this is nil: keep the default
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("settings daily_goal_minutes: bad value %q: %w", value, err)
	}
	s.settings.DailyGoalMinutes = n
	return nil
}

// setDailyGoalMinutes upserts the daily focus goal and updates the in-memory
// copy. The caller must hold the lock.
func (s *Store) setDailyGoalMinutes(n int) error {
	if err := s.setSetting("daily_goal_minutes", strconv.Itoa(n)); err != nil {
		return err
	}
	s.settings.DailyGoalMinutes = n
	return nil
}

// findTask returns the task with the given id, or nil.
func (s *Store) findTask(id string) *Task {
	for _, t := range s.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// findSubject returns the subject with the given id, or nil.
func (s *Store) findSubject(id string) *Subject {
	for _, sub := range s.subjects {
		if sub.ID == id {
			return sub
		}
	}
	return nil
}
