package notes

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/data"
)

// Note represents a user's note
type Note struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Pinned    bool      `json:"pinned,omitempty"`
	Archived  bool      `json:"archived,omitempty"`
	Color     string    `json:"color,omitempty"` // Optional: yellow, green, blue, pink, purple, gray
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	mutex sync.RWMutex
	notes []*Note
)

// Load initializes the notes package
func Load() {
	loadNotes()
	app.Log("notes", "Loaded %d notes", len(notes))
}

func loadNotes() {
	mutex.Lock()
	defer mutex.Unlock()

	b, err := data.LoadFile("notes.json")
	if err != nil {
		notes = []*Note{}
		return
	}

	var loaded []*Note
	if err := json.Unmarshal(b, &loaded); err != nil {
		app.Log("notes", "Error loading notes: %v", err)
		notes = []*Note{}
		return
	}
	notes = loaded
}

func saveNotes() error {
	b, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return data.SaveFile("notes.json", string(b))
}

// CreateNote creates a new note for a user
func CreateNote(userID, title, content string, tags []string) (*Note, error) {
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()
	note := &Note{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		UserID:    userID,
		Title:     strings.TrimSpace(title),
		Content:   strings.TrimSpace(content),
		Tags:      normalizeTags(tags),
		CreatedAt: now,
		UpdatedAt: now,
	}

	notes = append(notes, note)
	if err := saveNotes(); err != nil {
		return nil, err
	}

	return note, nil
}

// QuickNote creates a note with just content (for agent use)
func QuickNote(userID, content string) (*Note, error) {
	return CreateNote(userID, "", content, nil)
}

// GetNote retrieves a note by ID
func GetNote(id, userID string) *Note {
	mutex.RLock()
	defer mutex.RUnlock()

	for _, n := range notes {
		if n.ID == id && n.UserID == userID {
			return n
		}
	}
	return nil
}

// UpdateNote updates an existing note
func UpdateNote(id, userID, title, content string, tags []string, pinned, archived bool, color string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for _, n := range notes {
		if n.ID == id && n.UserID == userID {
			n.Title = strings.TrimSpace(title)
			n.Content = strings.TrimSpace(content)
			n.Tags = normalizeTags(tags)
			n.Pinned = pinned
			n.Archived = archived
			n.Color = color
			n.UpdatedAt = time.Now()
			return saveNotes()
		}
	}
	return fmt.Errorf("note not found")
}

// DeleteNote removes a note
func DeleteNote(id, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for i, n := range notes {
		if n.ID == id && n.UserID == userID {
			notes = append(notes[:i], notes[i+1:]...)
			return saveNotes()
		}
	}
	return fmt.Errorf("note not found")
}

// ArchiveNote archives or unarchives a note
func ArchiveNote(id, userID string, archive bool) error {
	mutex.Lock()
	defer mutex.Unlock()

	for _, n := range notes {
		if n.ID == id && n.UserID == userID {
			n.Archived = archive
			n.UpdatedAt = time.Now()
			return saveNotes()
		}
	}
	return fmt.Errorf("note not found")
}

// PinNote pins or unpins a note
func PinNote(id, userID string, pin bool) error {
	mutex.Lock()
	defer mutex.Unlock()

	for _, n := range notes {
		if n.ID == id && n.UserID == userID {
			n.Pinned = pin
			n.UpdatedAt = time.Now()
			return saveNotes()
		}
	}
	return fmt.Errorf("note not found")
}

// ListNotes returns notes for a user with optional filters
func ListNotes(userID string, archived bool, tag string, limit int) []*Note {
	mutex.RLock()
	defer mutex.RUnlock()

	var result []*Note
	for _, n := range notes {
		if n.UserID != userID {
			continue
		}
		if n.Archived != archived {
			continue
		}
		if tag != "" && !hasTag(n.Tags, tag) {
			continue
		}
		result = append(result, n)
	}

	// Sort: pinned first, then by updated time
	sort.Slice(result, func(i, j int) bool {
		if result[i].Pinned != result[j].Pinned {
			return result[i].Pinned
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

// SearchNotes searches notes by content/title
func SearchNotes(userID, query string, limit int) []*Note {
	mutex.RLock()
	defer mutex.RUnlock()

	query = strings.ToLower(query)
	var result []*Note

	for _, n := range notes {
		if n.UserID != userID {
			continue
		}
		// Search in title and content
		if strings.Contains(strings.ToLower(n.Title), query) ||
			strings.Contains(strings.ToLower(n.Content), query) {
			result = append(result, n)
		}
	}

	// Sort by relevance (title match first) then by date
	sort.Slice(result, func(i, j int) bool {
		iTitle := strings.Contains(strings.ToLower(result[i].Title), query)
		jTitle := strings.Contains(strings.ToLower(result[j].Title), query)
		if iTitle != jTitle {
			return iTitle
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

// GetAllTags returns all unique tags for a user
func GetAllTags(userID string) []string {
	mutex.RLock()
	defer mutex.RUnlock()

	tagSet := make(map[string]bool)
	for _, n := range notes {
		if n.UserID == userID {
			for _, t := range n.Tags {
				tagSet[t] = true
			}
		}
	}

	var tags []string
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// Helper functions
func normalizeTags(tags []string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

func hasTag(tags []string, tag string) bool {
	tag = strings.ToLower(tag)
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func parseTags(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	return normalizeTags(parts)
}
