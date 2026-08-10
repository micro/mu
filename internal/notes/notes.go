// Package notes is the store behind a person's notes: a title, what is under
// it, and when it was written.
//
// It was called memory, then cache, and both names described the machinery
// rather than the thing. What is actually kept here is what anybody would call
// a note — you write one, you read it back, you throw it away — and the agent
// writing one from a conversation is the same act as a person typing one.
//
// Nothing expires and nothing is evicted. The on-disk file is still
// memory.json and its fields are still key/value: data outlives the names we
// give it, and renaming a file to match a label is not worth a migration.
package notes

import (
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// Entry is one note.
type Entry struct {
	Title     string    `json:"key"`
	Text      string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaxPerUser caps the store to prevent unbounded growth.
const MaxPerUser = 50

var (
	mu    sync.RWMutex
	store = map[string][]*Entry{} // userID → notes
)

func init() {
	data.LoadJSON("memory.json", &store)
}

func save() {
	data.SaveJSON("memory.json", store)
}

// Add writes a note. A title that already exists is rewritten rather than
// duplicated — a note is addressed by its title, which is what makes "remember
// that I'm in London" idempotent however many times it is said.
func Add(userID, title, text string) {
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if title == "" || text == "" {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	entries := store[userID]
	now := time.Now()

	for _, e := range entries {
		if strings.EqualFold(e.Title, title) {
			e.Text = text
			e.UpdatedAt = now
			save()
			return
		}
	}

	entries = append(entries, &Entry{
		Title:     title,
		Text:      text,
		CreatedAt: now,
		UpdatedAt: now,
	})

	if len(entries) > MaxPerUser {
		entries = entries[len(entries)-MaxPerUser:]
	}

	store[userID] = entries
	save()
}

// Get returns one note's text by title.
func Get(userID, title string) string {
	mu.RLock()
	defer mu.RUnlock()

	for _, e := range store[userID] {
		if strings.EqualFold(e.Title, title) {
			return e.Text
		}
	}
	return ""
}

// All returns every note a user has, oldest first.
func All(userID string) []*Entry {
	mu.RLock()
	defer mu.RUnlock()

	entries := store[userID]
	result := make([]*Entry, len(entries))
	for i, entry := range entries {
		copy := *entry
		result[i] = &copy
	}
	return result
}

// ForContext returns the notes formatted for the agent's system prompt.
func ForContext(userID string) string {
	mu.RLock()
	defer mu.RUnlock()

	entries := store[userID]
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("- ")
		sb.WriteString(e.Title)
		sb.WriteString(": ")
		sb.WriteString(e.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ForScopedContext returns the notes relevant to one agent's scope: every
// global note (no ":" in the title) plus the ones titled for that scope.
func ForScopedContext(userID, scope string) string {
	mu.RLock()
	defer mu.RUnlock()

	entries := store[userID]
	if len(entries) == 0 {
		return ""
	}

	prefix := scope + ":"
	var sb strings.Builder
	for _, e := range entries {
		if !strings.Contains(e.Title, ":") || strings.HasPrefix(e.Title, prefix) {
			sb.WriteString("- ")
			sb.WriteString(strings.TrimPrefix(e.Title, prefix))
			sb.WriteString(": ")
			sb.WriteString(e.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// Delete removes one note by title.
func Delete(userID, title string) {
	mu.Lock()
	defer mu.Unlock()

	entries := store[userID]
	var kept []*Entry
	for _, e := range entries {
		if !strings.EqualFold(e.Title, title) {
			kept = append(kept, e)
		}
	}
	store[userID] = kept
	save()
}

// Clear removes every note a user has (account deletion).
func Clear(userID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(store, userID)
	save()
}
