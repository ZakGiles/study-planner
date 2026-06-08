package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Store holds all topics and persists them to a JSON file on disk.
type Store struct {
	mu     sync.Mutex
	path   string
	topics []*Topic
}

// NewStore creates a store backed by data.json inside the user's config
// directory (e.g. ~/Library/Application Support/study-planner on macOS) and
// loads any existing data.
func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(dir, "study-planner")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		path:   filepath.Join(appDir, "data.json"),
		topics: []*Topic{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads topics from disk. A missing file is treated as an empty store.
func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var topics []*Topic
	if err := json.Unmarshal(data, &topics); err != nil {
		return err
	}
	if topics == nil {
		topics = []*Topic{}
	}
	for _, t := range topics {
		if t.Sessions == nil {
			t.Sessions = []*Session{}
		}
	}
	s.topics = topics
	return nil
}

// save writes the current topics to disk atomically.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.topics, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
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
