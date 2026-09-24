package auth

import (
	"fmt"
	"mu/internal/data"
)

func CodexPreviewEnabled(id string) bool {
	a, ok := snapshot(id)
	return ok && a.Admin && !a.Banned && a.CodexPreview
}

// SetCodexPreview persists a self-service pilot preference atomically.
func SetCodexPreview(id string, enabled bool) error {
	mutex.Lock()
	defer mutex.Unlock()
	a, ok := accounts[id]
	if !ok || !a.Admin || a.Banned {
		return fmt.Errorf("admin account required")
	}
	updated := *a
	updated.CodexPreview = enabled
	next := make(map[string]*Account, len(accounts))
	for k, v := range accounts {
		next[k] = v
	}
	next[id] = &updated
	if err := data.SaveJSON("accounts.json", next); err != nil {
		return err
	}
	accounts = next
	return nil
}
