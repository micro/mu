package app

import (
	"encoding/json"
	"sync"
	"time"

	"mu/internal/data"
)

// UserPrefs stores per-user content preferences (saves, dismissals, blocks)
type UserPrefs struct {
	Saved     map[string]time.Time `json:"saved"`     // "type:id" → saved time
	Dismissed map[string]time.Time `json:"dismissed"` // "type:id" → dismissed time
	Blocked   map[string]time.Time `json:"blocked"`   // userID → blocked time
}

var (
	prefsMu sync.RWMutex
	prefs   = map[string]*UserPrefs{} // userID → prefs
)

func init() {
	b, _ := data.LoadFile("prefs.json")
	if len(b) > 0 {
		json.Unmarshal(b, &prefs)
	}
}

func savePrefs() {
	data.SaveJSON("prefs.json", prefs)
}

func getUserPrefs(userID string) *UserPrefs {
	p, ok := prefs[userID]
	if !ok {
		p = &UserPrefs{
			Saved:     map[string]time.Time{},
			Dismissed: map[string]time.Time{},
			Blocked:   map[string]time.Time{},
		}
		prefs[userID] = p
	}
	return p
}

// SaveItem bookmarks a content item for the user
func SaveItem(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	p.Saved[contentType+":"+contentID] = time.Now()
	savePrefs()
}

// UnsaveItem removes a bookmark
func UnsaveItem(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	delete(p.Saved, contentType+":"+contentID)
	savePrefs()
}

// DismissItem hides a content item from the user's view
func DismissItem(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	p.Dismissed[contentType+":"+contentID] = time.Now()
	savePrefs()
}

// IsDismissed checks if the user has dismissed this item
func IsDismissed(userID, contentType, contentID string) bool {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return false
	}
	_, dismissed := p.Dismissed[contentType+":"+contentID]
	return dismissed
}

// BlockUser blocks all content from a specific user
func BlockUser(userID, blockedUserID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	p.Blocked[blockedUserID] = time.Now()
	savePrefs()
}

// UnblockUser unblocks a user
func UnblockUser(userID, blockedUserID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	delete(p.Blocked, blockedUserID)
	savePrefs()
}

// IsBlocked checks if the user has blocked this author
func IsBlocked(userID, authorID string) bool {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return false
	}
	_, blocked := p.Blocked[authorID]
	return blocked
}

// SavedItems returns all saved item keys for a user
func SavedItems(userID string) map[string]time.Time {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return nil
	}
	return p.Saved
}

// BlockedUsers returns all blocked user IDs for a user
func BlockedUsers(userID string) map[string]time.Time {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	p, ok := prefs[userID]
	if !ok {
		return nil
	}
	return p.Blocked
}

// ClearUserPrefs removes all preferences for a deleted user.
func ClearUserPrefs(userID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	delete(prefs, userID)
	savePrefs()
}

// HiddenItems returns what this account has hidden from its own view,
// keyed "type:id".
//
// There was no getter: hiding was write-only, so an item could be dismissed and
// never found again — no page listed them and nothing could undo one. IsDismissed
// answered about a single item you already had in your hand, which is the wrong
// shape for "what did I hide".
func HiddenItems(userID string) map[string]time.Time {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	out := map[string]time.Time{}
	p, ok := prefs[userID]
	if !ok {
		return out
	}
	for k, t := range p.Dismissed {
		out[k] = t
	}
	return out
}

// Unhide puts a hidden item back in the caller's view.
func Unhide(userID, contentType, contentID string) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p := getUserPrefs(userID)
	delete(p.Dismissed, contentType+":"+contentID)
	savePrefs()
}
