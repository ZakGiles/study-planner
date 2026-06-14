package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store holds all topics in memory and persists them to a SQLite database.
// The in-memory slice stays the authoritative working copy that app.go mutates;
// save() rewrites the whole graph to SQLite in one transaction.
type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	jsonPath string // legacy data.json, used for one-time import and kept as backup
	topics   []*Topic
}

// schema is the database layout. There is deliberately no UNIQUE(topic_id, date)
// on sessions: a done (historical) session and a pending session may share a day.
// The "at most one pending session per (topic, date)" invariant is enforced in
// app.go, not the database.
const schema = `
CREATE TABLE IF NOT EXISTS topics (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  color       TEXT NOT NULL DEFAULT '',
  archived    INTEGER NOT NULL DEFAULT 0,
  adaptive    INTEGER NOT NULL DEFAULT 0,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id           TEXT PRIMARY KEY,
  topic_id     TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  done         INTEGER NOT NULL DEFAULT 0,
  completed_at TEXT
);
CREATE TABLE IF NOT EXISTS topic_tags (
  topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  tag      TEXT NOT NULL,
  position INTEGER NOT NULL,
  PRIMARY KEY (topic_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_sessions_topic ON sessions(topic_id);
CREATE INDEX IF NOT EXISTS idx_sessions_date  ON sessions(date);
PRAGMA user_version = 1;
`

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
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{
		db:       db,
		jsonPath: filepath.Join(filepath.Dir(dbPath), "data.json"),
		topics:   []*Topic{},
	}
	if err := s.importLegacyJSON(); err != nil {
		// Import is best-effort: a corrupt backup must not block startup.
		log.Printf("study-planner: legacy data.json import skipped: %v", err)
	}
	if err := s.load(); err != nil {
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

// importLegacyJSON, on first run only (no topics yet), imports a legacy data.json
// into the database and leaves the JSON file in place as a backup.
func (s *Store) importLegacyJSON() error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM topics").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // database already populated; nothing to import
	}
	topics, err := readLegacyJSON(s.jsonPath)
	if err != nil {
		return err
	}
	if topics == nil {
		return nil // no data.json present
	}
	s.topics = topics
	return s.save()
}

// readLegacyJSON reads and normalizes topics from a legacy data.json. A missing
// file returns (nil, nil); an empty file returns an empty slice.
func readLegacyJSON(path string) ([]*Topic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []*Topic{}, nil
	}
	var topics []*Topic
	if err := json.Unmarshal(data, &topics); err != nil {
		return nil, err
	}
	if topics == nil {
		topics = []*Topic{}
	}
	return topics, nil
}

// load reads all topics, sessions and tags from the database into memory.
func (s *Store) load() error {
	rows, err := s.db.Query(
		`SELECT id, name, description, color, archived, adaptive, sort_order, created_at
		 FROM topics ORDER BY sort_order, created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var topics []*Topic
	byID := make(map[string]*Topic)
	for rows.Next() {
		t := &Topic{Tags: []string{}, Sessions: []*Session{}}
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Color,
			&t.Archived, &t.Adaptive, &t.Order, &createdAt); err != nil {
			return err
		}
		if t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return fmt.Errorf("topic %s: bad created_at %q: %w", t.ID, createdAt, err)
		}
		topics = append(topics, t)
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

	if topics == nil {
		topics = []*Topic{}
	}
	normalizeOrder(topics)
	s.topics = topics
	return nil
}

// loadTags attaches tags (in stored order) to the topics in byID.
func (s *Store) loadTags(byID map[string]*Topic) error {
	rows, err := s.db.Query(`SELECT topic_id, tag FROM topic_tags ORDER BY topic_id, position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var topicID, tag string
		if err := rows.Scan(&topicID, &tag); err != nil {
			return err
		}
		if t := byID[topicID]; t != nil {
			t.Tags = append(t.Tags, tag)
		}
	}
	return rows.Err()
}

// loadSessions attaches sessions to the topics in byID. Ordering is irrelevant
// here because snapshot() re-sorts sessions by date before serving them.
func (s *Store) loadSessions(byID map[string]*Topic) error {
	rows, err := s.db.Query(`SELECT id, topic_id, date, done, completed_at FROM sessions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var topicID, completedAt string
		var nullCompleted sql.NullString
		sess := &Session{}
		if err := rows.Scan(&sess.ID, &topicID, &sess.Date, &sess.Done, &nullCompleted); err != nil {
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
		if t := byID[topicID]; t != nil {
			t.Sessions = append(t.Sessions, sess)
		}
	}
	return rows.Err()
}

// save rewrites the entire in-memory graph to the database in one transaction.
// Deleting all topics cascades to sessions and tags (foreign_keys=1), so the
// re-insert below is a clean full replacement, matching the previous JSON save.
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

	if _, err = tx.Exec(`DELETE FROM topics`); err != nil {
		return err
	}

	topicStmt, err := tx.Prepare(
		`INSERT INTO topics (id, name, description, color, archived, adaptive, sort_order, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer topicStmt.Close()
	tagStmt, err := tx.Prepare(`INSERT INTO topic_tags (topic_id, tag, position) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer tagStmt.Close()
	sessStmt, err := tx.Prepare(
		`INSERT INTO sessions (id, topic_id, date, done, completed_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer sessStmt.Close()

	for _, t := range s.topics {
		if _, err = topicStmt.Exec(t.ID, t.Name, t.Description, t.Color,
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

// find returns the topic with the given id, or nil.
func (s *Store) find(id string) *Topic {
	for _, t := range s.topics {
		if t.ID == id {
			return t
		}
	}
	return nil
}
