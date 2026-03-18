// Package flag provides content flagging, hiding, and moderation primitives.
//
// Content is flagged by users or the system. After 3 flags, content is
// automatically hidden. Admins can immediately hide or approve content.
package flag

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"mu/data"
)

// FlaggedItem tracks flags on a piece of content.
type FlaggedItem struct {
	ContentType string    `json:"content_type"` // "post", "thread", "news", "video"
	ContentID   string    `json:"content_id"`
	FlagCount   int       `json:"flag_count"`
	Flagged     bool      `json:"flagged"`    // Hidden from public view
	FlaggedBy   []string  `json:"flagged_by"` // Usernames who flagged
	FlaggedAt   time.Time `json:"flagged_at"` // First flag timestamp
}

// ContentDeleter interface — each content type implements this.
type ContentDeleter interface {
	Delete(id string) error
	Get(id string) interface{}
	RefreshCache()
}

// Analyzer checks content and returns a classification string.
type Analyzer interface {
	Analyze(prompt, question string) (string, error)
}

var (
	mu       sync.RWMutex
	flags    = make(map[string]*FlaggedItem) // key: contentType:contentID
	deleters = make(map[string]ContentDeleter)
	analyzer Analyzer
)

// Load reads persisted flags from disk.
func Load() {
	b, err := data.LoadFile("flags.json")
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
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
func GetDeleter(contentType string) ContentDeleter {
	return deleters[contentType]
}

// SetAnalyzer sets the content analyzer for auto-moderation.
func SetAnalyzer(a Analyzer) {
	analyzer = a
}

// CheckContent analyzes content using the analyzer and flags if suspicious.
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

// Add adds a flag to content. Returns new flag count, whether already flagged by this user, error.
func Add(contentType, contentID, username string) (int, bool, error) {
	key := contentType + ":" + contentID

	mu.Lock()
	defer mu.Unlock()

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

	// Check if user already flagged
	for _, flagger := range item.FlaggedBy {
		if flagger == username {
			return item.FlagCount, true, nil
		}
	}

	// Add flag
	item.FlaggedBy = append(item.FlaggedBy, username)
	item.FlagCount++

	// Auto-hide after 3 flags
	if item.FlagCount >= 3 {
		item.Flagged = true
	}

	saveUnlocked()
	return item.FlagCount, false, nil
}

// GetCount returns the flag count for content.
func GetCount(contentType, contentID string) int {
	count, _ := GetFlags(contentType, contentID)
	return count
}

// GetFlags returns flag info for content (flagCount, isFlagged).
func GetFlags(contentType, contentID string) (int, bool) {
	key := contentType + ":" + contentID

	mu.RLock()
	defer mu.RUnlock()

	if item, exists := flags[key]; exists {
		return item.FlagCount, item.Flagged
	}
	return 0, false
}

// GetItem returns full flag details.
func GetItem(contentType, contentID string) *FlaggedItem {
	key := contentType + ":" + contentID

	mu.RLock()
	defer mu.RUnlock()

	if item, exists := flags[key]; exists {
		return item
	}
	return nil
}

// GetAll returns all flagged items.
func GetAll() []*FlaggedItem {
	mu.RLock()
	defer mu.RUnlock()

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

	mu.Lock()
	delete(flags, key)
	err := saveUnlocked()
	mu.Unlock()

	if err != nil {
		return err
	}

	// Refresh the content cache after unlocking to avoid deadlock
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

// AdminFlag immediately hides content.
func AdminFlag(contentType, contentID, username string) error {
	key := contentType + ":" + contentID

	mu.Lock()
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
	mu.Unlock()

	if err != nil {
		return err
	}

	// Refresh cache immediately
	if deleter, ok := deleters[contentType]; ok {
		go deleter.RefreshCache()
	}
	return nil
}

// Delete removes both the flag and the content.
func Delete(contentType, contentID string) error {
	key := contentType + ":" + contentID

	mu.Lock()
	delete(flags, key)
	err := saveUnlocked()
	mu.Unlock()

	if err != nil {
		return err
	}

	// Delete the actual content
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
