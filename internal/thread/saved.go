package thread

import "fmt"

// SetSaved bookmarks an owned conversation without changing its read state.
func SetSaved(account, id string, saved bool) error {
	ensure()
	mu.Lock()
	defer mu.Unlock()
	t := resolveUnlocked(account, id)
	if t == nil || t.Account != account {
		return fmt.Errorf("conversation not found")
	}
	t.Saved = saved
	save()
	return nil
}

// Sent reports whether the owner has written on this conversation. Incoming
// correspondents have From set; agent replies have RoleAgent.
func Sent(account, id string) bool {
	ensure()
	mu.RLock()
	defer mu.RUnlock()
	t := resolveUnlocked(account, id)
	if t == nil || t.Account != account {
		return false
	}
	for _, m := range messages[t.ID] {
		if m.Role == RolePerson && m.From == "" {
			return true
		}
	}
	return false
}
