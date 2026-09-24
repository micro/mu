package data

import (
	"encoding/json"
	"mu/internal/event"
)

// CommitJSON persists service state and its domain events in one recoverable
// transaction. Source ownership and event meaning stay with the caller.
func CommitJSON(key string, value interface{}, records ...event.Record) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := dataPath(key)
	if err != nil {
		return err
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	if err := keepIfShrinking(key, file, len(b)); err != nil {
		return err
	}
	return event.Commit(map[string][]byte{key: b}, records...)
}
