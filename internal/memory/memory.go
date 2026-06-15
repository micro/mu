// Package memory provides persistent per-user memory for the AI agent.
// The agent remembers preferences, interests, and facts about the user
// across sessions. Memory is stored as simple key-value notes.
package memory

import (
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// Entry is a single thing the agent remembers about a user.
type Entry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaxEntriesPerUser caps memory to prevent unbounded growth.
const MaxEntriesPerUser = 50

var (
	mu      sync.RWMutex
	store   = map[string][]*Entry{} // userID → entries
)

func init() {
	data.LoadJSON("memory.json", &store)
}

func save() {
	data.SaveJSON("memory.json", store)
}

// Set stores or updates a memory entry for a user.
func Set(userID, key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	entries := store[userID]
	now := time.Now()

	// Update if key exists.
	for _, e := range entries {
		if strings.EqualFold(e.Key, key) {
			e.Value = value
			e.UpdatedAt = now
			save()
			return
		}
	}

	// Add new entry.
	entries = append(entries, &Entry{
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Cap at max.
	if len(entries) > MaxEntriesPerUser {
		entries = entries[len(entries)-MaxEntriesPerUser:]
	}

	store[userID] = entries
	save()
}

// Get retrieves a specific memory by key.
func Get(userID, key string) string {
	mu.RLock()
	defer mu.RUnlock()

	for _, e := range store[userID] {
		if strings.EqualFold(e.Key, key) {
			return e.Value
		}
	}
	return ""
}

// All returns all memory entries for a user.
func All(userID string) []*Entry {
	mu.RLock()
	defer mu.RUnlock()

	entries := store[userID]
	result := make([]*Entry, len(entries))
	copy(result, entries)
	return result
}

// ForContext returns a formatted string of all memories suitable for
// injecting into the agent's system prompt.
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
		sb.WriteString(e.Key)
		sb.WriteString(": ")
		sb.WriteString(e.Value)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ForScopedContext returns memory entries relevant to a specific agent scope.
// Includes all global entries (no ":" prefix) plus entries in the given scope.
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
		// Include global entries and scope-matching entries
		if !strings.Contains(e.Key, ":") || strings.HasPrefix(e.Key, prefix) {
			sb.WriteString("- ")
			sb.WriteString(strings.TrimPrefix(e.Key, prefix))
			sb.WriteString(": ")
			sb.WriteString(e.Value)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// Delete removes a memory entry.
func Delete(userID, key string) {
	mu.Lock()
	defer mu.Unlock()

	entries := store[userID]
	var kept []*Entry
	for _, e := range entries {
		if !strings.EqualFold(e.Key, key) {
			kept = append(kept, e)
		}
	}
	store[userID] = kept
	save()
}

// Clear removes all memory for a user (account deletion).
func Clear(userID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(store, userID)
	save()
}
