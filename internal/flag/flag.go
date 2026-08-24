// Package flag provides content moderation primitives (flagging, hiding,
// auto-moderation). It lives in internal/ because it is infrastructure used by
// multiple building blocks, not a feature itself.
package flag

import (
	"encoding/json"
	"sync"
	"time"

	"mu/internal/data"
)

// FlaggedItem represents a piece of content that has been flagged.
type FlaggedItem struct {
	ContentType string    `json:"content_type"` // "post", "thread", etc.
	ContentID   string    `json:"content_id"`
	FlagCount   int       `json:"flag_count"`
	Flagged     bool      `json:"flagged"`    // Hidden from public view
	FlaggedBy   []string  `json:"flagged_by"` // Usernames who flagged
	FlaggedAt   time.Time `json:"flagged_at"` // First flag timestamp
}

// ContentDeleter interface — each building block that supports moderation
// registers a deleter for its content type.
type ContentDeleter interface {
	Delete(id string) error
	Get(id string) interface{}
	RefreshCache()
}

var (
	mutex    sync.RWMutex
	flags    = make(map[string]*FlaggedItem)
	deleters = make(map[string]ContentDeleter)
)

// Load reads persisted flags from disk.
func Load() {
	b, err := data.LoadFile("flags.json")
	if err != nil {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()
	json.Unmarshal(b, &flags)
}

func saveUnlocked() error {
	return data.SaveJSON("flags.json", flags)
}

// RegisterDeleter registers a content type handler.
func RegisterDeleter(contentType string, deleter ContentDeleter) {
	deleters[contentType] = deleter
}

// Deleter returns the registered deleter for a content type.
func Deleter(contentType string) (ContentDeleter, bool) {
	d, ok := deleters[contentType]
	return d, ok
}

// No analyzer here, and no CheckContent.
//
// This package used to hold both: an `analyzer` function variable, a
// SetAnalyzer to fill it in, and a CheckContent that ran a model over a
// paragraph and decided it was spam. That is a judgement, and this is a
// record — every other function in this file answers a question about state.
//
// It also broke the layering in the way that is hardest to see. service/social,
// service/blog and service/apps called CheckContent, so three services were
// asking a model what their own answer should be; the variable was filled in
// by service/chat, so moderation for the whole instance depended on an
// unrelated service loading, and CheckContent opened by returning silently
// when it had not. "A function variable is an import the compiler cannot see."
//
// The services publish event.ContentPublished now and agent/moderate
// subscribes, which is the same shape as service/mail and agent/mail. What
// reaches this package is AdminFlag, exactly as it does when a person presses
// the flag button.

// Add adds a flag to content (returns new flag count, already flagged bool, error).
func Add(contentType, contentID, username string) (int, bool, error) {
	key := contentType + ":" + contentID

	mutex.Lock()
	defer mutex.Unlock()

	item, exists := flags[key]
	if !exists {
		item = &FlaggedItem{
			ContentType: contentType,
			ContentID:   contentID,
			FlagCount:   0,
			Flagged:     false,
			FlaggedBy:   []string{},
			FlaggedAt:   time.Now(),
		}
		flags[key] = item
	}

	for _, flagger := range item.FlaggedBy {
		if flagger == username {
			return item.FlagCount, true, nil
		}
	}

	item.FlaggedBy = append(item.FlaggedBy, username)
	item.FlagCount++

	if item.FlagCount >= 3 {
		item.Flagged = true
	}

	saveUnlocked()
	return item.FlagCount, false, nil
}

// Count returns flag count for content.
func Count(contentType, contentID string) int {
	count, _ := Flags(contentType, contentID)
	return count
}

// Flags returns flag info for content (flagCount, isFlagged).
func Flags(contentType, contentID string) (int, bool) {
	key := contentType + ":" + contentID
	mutex.RLock()
	defer mutex.RUnlock()
	if item, exists := flags[key]; exists {
		return item.FlagCount, item.Flagged
	}
	return 0, false
}

// Item returns full flag details.
func Item(contentType, contentID string) *FlaggedItem {
	key := contentType + ":" + contentID
	mutex.RLock()
	defer mutex.RUnlock()
	if item, exists := flags[key]; exists {
		return item
	}
	return nil
}

// All returns all flagged items.
func All() []*FlaggedItem {
	mutex.RLock()
	defer mutex.RUnlock()
	var items []*FlaggedItem
	for _, item := range flags {
		if item.FlagCount > 0 {
			items = append(items, item)
		}
	}
	return items
}

// Approve clears flags for content.
func Approve(contentType, contentID string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	delete(flags, key)
	err := saveUnlocked()
	mutex.Unlock()

	if err != nil {
		return err
	}

	if deleter, ok := deleters[contentType]; ok {
		deleter.RefreshCache()
	}

	return nil
}

// IsHidden checks if content is flagged/hidden.
func IsHidden(contentType, contentID string) bool {
	_, flagged := Flags(contentType, contentID)
	return flagged
}

// AdminFlag immediately hides content (for admin use).
func AdminFlag(contentType, contentID, username string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	adminFlagger := username + " (admin)"
	if item, exists := flags[key]; exists {
		item.FlagCount = 3
		item.Flagged = true
		if !contains(item.FlaggedBy, adminFlagger) {
			item.FlaggedBy = append(item.FlaggedBy, adminFlagger)
		}
	} else {
		flags[key] = &FlaggedItem{
			ContentType: contentType,
			ContentID:   contentID,
			FlagCount:   3,
			Flagged:     true,
			FlaggedBy:   []string{adminFlagger},
			FlaggedAt:   time.Now(),
		}
	}
	err := saveUnlocked()
	mutex.Unlock()

	if err != nil {
		return err
	}

	if deleter, ok := deleters[contentType]; ok {
		go deleter.RefreshCache()
	}

	return nil
}

// Delete removes both the flag and the content.
func Delete(contentType, contentID string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	delete(flags, key)
	err := saveUnlocked()
	mutex.Unlock()

	if err != nil {
		return err
	}

	if deleter, ok := deleters[contentType]; ok {
		if err := deleter.Delete(contentID); err != nil {
			return err
		}
		go deleter.RefreshCache()
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// PostContent represents post data for display in moderation views.
type PostContent struct {
	ID        string
	Title     string
	Content   string
	Author    string
	AuthorID  string
	CreatedAt time.Time
}
