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
		if topics[i].Order != topics[j].Order {
			return topics[i].Order < topics[j].Order
		}
		return topics[i].CreatedAt.Before(topics[j].CreatedAt)
	})
	return topics
}

// mutate runs fn under the store lock, persists, and returns the new
// snapshot. fn returning an error skips the save.
func (a *App) mutate(fn func() error) ([]*Topic, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if err := fn(); err != nil {
		return nil, err
	}
	if err := a.store.save(); err != nil {
		return nil, err
	}
	return a.snapshot(), nil
}

// mutateTopic locates a topic and applies fn to it.
func (a *App) mutateTopic(id string, fn func(*Topic) error) ([]*Topic, error) {
	return a.mutate(func() error {
		t := a.store.find(id)
		if t == nil {
			return errors.New("topic not found")
		}
		return fn(t)
	})
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
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("topic name is required")
	}
	return a.mutate(func() error {
		a.store.topics = append(a.store.topics, &Topic{
			ID:          uuid.NewString(),
			Name:        name,
			Description: strings.TrimSpace(description),
			Color:       TopicColors[len(a.store.topics)%len(TopicColors)],
			Tags:        []string{},
			Order:       len(a.store.topics),
			CreatedAt:   time.Now(),
			Sessions:    []*Session{},
		})
		return nil
	})
}

// UpdateTopic edits an existing topic's name, description and tags.
func (a *App) UpdateTopic(id, name, description string, tags []string) ([]*Topic, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("topic name is required")
	}
	return a.mutateTopic(id, func(t *Topic) error {
		t.Name = name
		t.Description = strings.TrimSpace(description)
		t.Tags = normalizeTags(tags)
		return nil
	})
}

// SetTopicColor sets a topic's palette colour. An empty string resets it to the
// default accent; any other value must be a known palette token.
func (a *App) SetTopicColor(id, color string) ([]*Topic, error) {
	if !validColor(color) {
		return nil, errors.New("unknown colour")
	}
	return a.mutateTopic(id, func(t *Topic) error {
		t.Color = color
		return nil
	})
}

// SetTopicArchived archives or unarchives a topic.
func (a *App) SetTopicArchived(id string, archived bool) ([]*Topic, error) {
	return a.mutateTopic(id, func(t *Topic) error {
		t.Archived = archived
		return nil
	})
}

// ReorderTopics applies a new manual order. orderedIDs lists topic ids in the
// desired order; any topic not included keeps its relative order after them
// (e.g. archived topics that are hidden from the reorderable list).
func (a *App) ReorderTopics(orderedIDs []string) ([]*Topic, error) {
	return a.mutate(func() error {
		pos := make(map[string]int, len(orderedIDs))
		for i, id := range orderedIDs {
			pos[id] = i
		}
		// Establish the current relative order first so unlisted topics keep it.
		sort.SliceStable(a.store.topics, func(i, j int) bool {
			return a.store.topics[i].Order < a.store.topics[j].Order
		})
		next := len(orderedIDs)
		for _, t := range a.store.topics {
			if p, ok := pos[t.ID]; ok {
				t.Order = p
			} else {
				t.Order = next
				next++
			}
		}
		normalizeOrder(a.store.topics)
		return nil
	})
}

// DeleteTopic removes a topic and all of its sessions.
func (a *App) DeleteTopic(id string) ([]*Topic, error) {
	return a.mutate(func() error {
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
			return errors.New("topic not found")
		}
		a.store.topics = kept
		normalizeOrder(a.store.topics)
		return nil
	})
}

// AddSession adds a single, manually-chosen study date to a topic.
func (a *App) AddSession(topicID, date string) ([]*Topic, error) {
	date = strings.TrimSpace(date)
	if _, err := time.Parse(dateLayout, date); err != nil {
		return nil, errors.New("date must be in YYYY-MM-DD format")
	}
	return a.mutateTopic(topicID, func(t *Topic) error {
		t.addDates([]string{date})
		return nil
	})
}

// AddSpacedSessions regenerates a topic's spaced-repetition schedule from a
// start date and a set of day offsets. Any existing sessions (including their
// done state) are cleared first, so the result is exactly the new schedule; the
// frontend confirms this with the user before calling. If intervals is empty the
// default schedule (0, 1, 3, 7, 14, 30 days) is used.
func (a *App) AddSpacedSessions(topicID, startDate string, intervals []int) ([]*Topic, error) {
	startDate = strings.TrimSpace(startDate)
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return nil, errors.New("start date must be in YYYY-MM-DD format")
	}
	if len(intervals) == 0 {
		intervals = DefaultIntervals
	}
	return a.mutateTopic(topicID, func(t *Topic) error {
		t.Sessions = []*Session{}
		t.addDates(spacedDates(start, intervals))
		return nil
	})
}

// DeleteSession removes a single study date from a topic.
func (a *App) DeleteSession(topicID, sessionID string) ([]*Topic, error) {
	return a.mutateTopic(topicID, func(t *Topic) error {
		kept := t.Sessions[:0]
		for _, s := range t.Sessions {
			if s.ID == sessionID {
				continue
			}
			kept = append(kept, s)
		}
		t.Sessions = kept
		return nil
	})
}

// ToggleSession flips the done state of a study session.
func (a *App) ToggleSession(topicID, sessionID string) ([]*Topic, error) {
	return a.mutateTopic(topicID, func(t *Topic) error {
		for _, s := range t.Sessions {
			if s.ID == sessionID {
				s.Done = !s.Done
				return nil
			}
		}
		return errors.New("session not found")
	})
}
