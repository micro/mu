// Package settings provides persistent configuration that can be edited
// from the admin UI. Values are stored in settings.json. Environment
// variables take precedence when set.
package settings

import (
	"os"
	"sync"

	"mu/internal/data"
)

var (
	mu     sync.RWMutex
	values = map[string]string{}
)

func Load() {
	loaded := map[string]string{}
	if err := data.LoadJSON("settings.json", &loaded); err != nil {
		return
	}

	mu.Lock()
	values = loaded
	mu.Unlock()
}

// Get returns the value for a key. Environment variable takes precedence
// over the stored value.
func Get(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	mu.RLock()
	defer mu.RUnlock()
	return values[key]
}

// Set stores a value persistently. Does not affect the environment.
func Set(key, value string) {
	_ = Store(key, value)
}

// Store changes a setting only if it can also be persisted.
func Store(key, value string) error {
	mu.Lock()
	defer mu.Unlock()
	updated := make(map[string]string, len(values)+1)
	for k, v := range values {
		updated[k] = v
	}
	if value == "" {
		delete(updated, key)
	} else {
		updated[key] = value
	}
	if err := data.SaveJSON("settings.json", updated); err != nil {
		return err
	}
	values = updated
	return nil
}

// IsSet returns true if the key has a value (from env or stored).
func IsSet(key string) bool {
	return Get(key) != ""
}

// Source returns where the value comes from: "env", "saved", or "".
func Source(key string) string {
	if os.Getenv(key) != "" {
		return "env"
	}
	mu.RLock()
	defer mu.RUnlock()
	if values[key] != "" {
		return "saved"
	}
	return ""
}

// All returns all stored settings (not env vars).
func All() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	result := make(map[string]string, len(values))
	for k, v := range values {
		result[k] = v
	}
	return result
}
