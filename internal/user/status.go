package user

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"mu/internal/data"
	"mu/internal/event"
)

// Status is the line somebody chose to display on their profile.
func Status(id string) string {
	profileMutex.RLock()
	defer profileMutex.RUnlock()
	if p := profiles[id]; p != nil {
		return p.Status
	}
	return ""
}

func SetStatus(id, text string) error {
	text = strings.Join(strings.Fields(text), " ")
	if id == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 160 {
		return fmt.Errorf("status must be at most 160 characters")
	}
	profileMutex.Lock()
	if profiles == nil {
		profiles = map[string]*Profile{}
	}
	old := profiles[id]
	if old != nil && old.Status == text {
		profileMutex.Unlock()
		return nil
	}
	profiles[id] = &Profile{UserID: id, Status: text, UpdatedAt: time.Now()}
	err := data.SaveJSON("profiles.json", profiles)
	if err != nil {
		if old == nil {
			delete(profiles, id)
		} else {
			profiles[id] = old
		}
	}
	profileMutex.Unlock()
	if err != nil {
		return err
	}
	line := "@" + id + ": " + text
	if text == "" {
		line = "@" + id + " cleared their status"
	}
	event.Announce("users", line, "/@"+id, "")
	return nil
}

func DeleteProfile(id string) {
	profileMutex.Lock()
	defer profileMutex.Unlock()
	delete(profiles, id)
	data.SaveJSON("profiles.json", profiles) //nolint:errcheck
}
