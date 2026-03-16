// Package moderation provides content moderation primitives (flagging, hiding,
// auto-moderation). It lives in internal/ because it is infrastructure used by
// multiple building blocks, not a feature itself.
package moderation

import (
	"encoding/json"
	"fmt"
	"strings"
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

// LLMAnalyzer interface for AI-powered content moderation.
type LLMAnalyzer interface {
	Analyze(prompt, question string) (string, error)
}

var (
	mutex    sync.RWMutex
	flags    = make(map[string]*FlaggedItem)
	deleters = make(map[string]ContentDeleter)
	analyzer LLMAnalyzer
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

// GetDeleter returns the registered deleter for a content type.
func GetDeleter(contentType string) (ContentDeleter, bool) {
	d, ok := deleters[contentType]
	return d, ok
}

// SetAnalyzer sets the LLM analyzer for content moderation.
func SetAnalyzer(a LLMAnalyzer) {
	analyzer = a
}

// CheckContent analyzes content using LLM and flags if suspicious.
func CheckContent(contentType, itemID, title, content string) {
	if analyzer == nil {
		return
	}

	prompt := `You are a content moderator for a community that values purposeful, respectful discussion. Every post should be meaningful — this is not a place to waste time.

Classify the content with ONLY ONE WORD:
- SPAM (promotional spam, advertising, repetitive junk)
- TEST (test posts like "test", "hello world", etc.)
- LOW_QUALITY (low-effort content, memes, nonsensical, no substance)
- HARMFUL (gossip, backbiting, slander, personal attacks, mocking others, trolling)
- OK (meaningful, on-topic, respectful content)

Community principles:
- Stay on topic and contribute something meaningful
- Be respectful — disagree with ideas, not people
- No gossip or backbiting (speaking ill of someone behind their back)
- No personal attacks, mockery, or belittling
- Religious and political discussion is welcome when done with sincerity and good manners

Respond with just the single word.`

	question := fmt.Sprintf("Title: %s\n\nContent: %s", title, content)

	resp, err := analyzer.Analyze(prompt, question)
	if err != nil {
		fmt.Printf("Moderation analysis error: %v\n", err)
		return
	}

	resp = strings.TrimSpace(strings.ToUpper(resp))
	fmt.Printf("Content moderation: %s %s -> %s\n", contentType, itemID, resp)

	if resp == "SPAM" || resp == "TEST" || resp == "LOW_QUALITY" || resp == "HARMFUL" {
		Add(contentType, itemID, "system")
		fmt.Printf("Auto-flagged %s: %s (reason: %s)\n", contentType, itemID, resp)
	}
}

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

// GetCount returns flag count for content.
func GetCount(contentType, contentID string) int {
	count, _ := GetFlags(contentType, contentID)
	return count
}

// GetFlags returns flag info for content (flagCount, isFlagged).
func GetFlags(contentType, contentID string) (int, bool) {
	key := contentType + ":" + contentID
	mutex.RLock()
	defer mutex.RUnlock()
	if item, exists := flags[key]; exists {
		return item.FlagCount, item.Flagged
	}
	return 0, false
}

// GetItem returns full flag details.
func GetItem(contentType, contentID string) *FlaggedItem {
	key := contentType + ":" + contentID
	mutex.RLock()
	defer mutex.RUnlock()
	if item, exists := flags[key]; exists {
		return item
	}
	return nil
}

// GetAll returns all flagged items.
func GetAll() []*FlaggedItem {
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
	_, flagged := GetFlags(contentType, contentID)
	return flagged
}

// AdminFlag immediately hides content (for admin use).
func AdminFlag(contentType, contentID, username string) error {
	key := contentType + ":" + contentID

	mutex.Lock()
	if item, exists := flags[key]; exists {
		item.FlagCount = 3
		item.Flagged = true
		if !contains(item.FlaggedBy, username) {
			item.FlaggedBy = append(item.FlaggedBy, username+" (admin)")
		}
	} else {
		flags[key] = &FlaggedItem{
			ContentType: contentType,
			ContentID:   contentID,
			FlagCount:   3,
			Flagged:     true,
			FlaggedBy:   []string{username + " (admin)"},
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
