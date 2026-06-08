package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// App is the Wails-bound application. Every mutating method returns the full,
// freshly-sorted list of topics so the frontend can replace its state in one go.
type App struct {
	ctx     context.Context
	store   *Store
	initErr error
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so we can call
// the runtime methods, and the on-disk store is loaded.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	store, err := NewStore()
	if err != nil {
		a.initErr = err
		return
	}
	a.store = store
}

// ready reports whether the store loaded successfully.
func (a *App) ready() error {
	if a.store == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errors.New("study planner is not ready yet")
	}
	return nil
}

// snapshot returns a sorted view of all topics. The caller must hold the lock.
func (a *App) snapshot() []*Topic {
	topics := a.store.topics
	for _, t := range topics {
		sortSessions(t.Sessions)
	}
	sort.SliceStable(topics, func(i, j int) bool {
		return topics[i].CreatedAt.Before(topics[j].CreatedAt)
	})
	return topics
}

// GetTopics returns all topics with their sessions.
func (a *App) GetTopics() ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	return a.snapshot(), nil
}

// AddTopic creates a new topic. The name is required; the description is optional.
func (a *App) AddTopic(name, description string) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("topic name is required")
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	a.store.topics = append(a.store.topics, &Topic{
		ID:          uuid.NewString(),
		Name:        name,
		Description: strings.TrimSpace(description),
		CreatedAt:   time.Now(),
		Sessions:    []*Session{},
	})
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// UpdateTopic edits an existing topic's name and description.
func (a *App) UpdateTopic(id, name, description string) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("topic name is required")
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	t := a.store.find(id)
	if t == nil {
		return nil, errors.New("topic not found")
	}
	t.Name = name
	t.Description = strings.TrimSpace(description)
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// DeleteTopic removes a topic and all of its sessions.
func (a *App) DeleteTopic(id string) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	kept := a.store.topics[:0]
	found := false
	for _, t := range a.store.topics {
		if t.ID == id {
			found = true
			continue
		}
		kept = append(kept, t)
	}
	if !found {
		return nil, errors.New("topic not found")
	}
	a.store.topics = kept
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// AddSession adds a single, manually-chosen study date to a topic.
func (a *App) AddSession(topicID, date string) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	date = strings.TrimSpace(date)
	if _, err := time.Parse(dateLayout, date); err != nil {
		return nil, errors.New("date must be in YYYY-MM-DD format")
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	t := a.store.find(topicID)
	if t == nil {
		return nil, errors.New("topic not found")
	}
	a.addDates(t, []string{date})
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// AddSpacedSessions regenerates a topic's spaced-repetition schedule from a
// start date and a set of day offsets. Any existing sessions (including their
// done state) are cleared first, so the result is exactly the new schedule; the
// frontend confirms this with the user before calling. If intervals is empty the
// default schedule (0, 1, 3, 7, 14, 30 days) is used.
func (a *App) AddSpacedSessions(topicID, startDate string, intervals []int) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	startDate = strings.TrimSpace(startDate)
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return nil, errors.New("start date must be in YYYY-MM-DD format")
	}
	if len(intervals) == 0 {
		intervals = DefaultIntervals
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	t := a.store.find(topicID)
	if t == nil {
		return nil, errors.New("topic not found")
	}
	t.Sessions = []*Session{}
	a.addDates(t, spacedDates(start, intervals))
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// addDates appends new sessions for any dates the topic does not already have.
// The caller must hold the lock.
func (a *App) addDates(t *Topic, dates []string) {
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

// DeleteSession removes a single study date from a topic.
func (a *App) DeleteSession(topicID, sessionID string) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	t := a.store.find(topicID)
	if t == nil {
		return nil, errors.New("topic not found")
	}
	kept := t.Sessions[:0]
	for _, s := range t.Sessions {
		if s.ID == sessionID {
			continue
		}
		kept = append(kept, s)
	}
	t.Sessions = kept
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// ToggleSession flips the done state of a study session.
func (a *App) ToggleSession(topicID, sessionID string) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	t := a.store.find(topicID)
	if t == nil {
		return nil, errors.New("topic not found")
	}
	for _, s := range t.Sessions {
		if s.ID == sessionID {
			s.Done = !s.Done
			if err := a.store.save(); err != nil {
				return nil, err
			}
			return a.snapshot(), nil
		}
	}
	return nil, errors.New("session not found")
}
