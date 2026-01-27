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

	// Index existing notes for RAG search
	go reindexAllNotes()

	// Subscribe to tag generation responses
	tagSub := data.Subscribe(data.EventTagGenerated)
	go func() {
		for event := range tagSub.Chan {
			noteID, okID := event.Data["note_id"].(string)
			userID, okUser := event.Data["user_id"].(string)
			tag, okTag := event.Data["tag"].(string)
			eventType, okType := event.Data["type"].(string)

			if okID && okUser && okTag && okType && eventType == "note" {
				app.Log("notes", "Received generated tag for note %s: %s", noteID, tag)
				addTagToNote(noteID, userID, tag)
			}
		}
	}()
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

	// Index for search (async)
	go indexNote(note)

	// Auto-tag if no tags provided
	if len(tags) == 0 && content != "" {
		go autoTagNote(note.ID, userID, title, content)
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
			app.Log("notes", "Saving note %s for user %s", id, userID)
			return saveNotes()
		}
	}
	app.Log("notes", "Note %s not found for user %s", id, userID)
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
	if limit <= 0 {
		limit = 20
	}

	// Try RAG search first for semantic matching
	ragResults := data.Search(query, limit*2, data.WithType("note"))
	
	var result []*Note
	seen := make(map[string]bool)

	// Filter RAG results to only this user's notes
	for _, entry := range ragResults {
		if entry.Metadata == nil {
			continue
		}
		entryUserID, _ := entry.Metadata["user_id"].(string)
		noteID, _ := entry.Metadata["note_id"].(string)
		
		if entryUserID != userID || noteID == "" {
			continue
		}
		
		if seen[noteID] {
			continue
		}
		seen[noteID] = true
		
		if note := GetNote(noteID, userID); note != nil {
			result = append(result, note)
		}
		
		if len(result) >= limit {
			break
		}
	}

	// Fallback to substring search if RAG found nothing
	if len(result) == 0 {
		mutex.RLock()
		queryLower := strings.ToLower(query)
		for _, n := range notes {
			if n.UserID != userID {
				continue
			}
			if strings.Contains(strings.ToLower(n.Title), queryLower) ||
				strings.Contains(strings.ToLower(n.Content), queryLower) {
				result = append(result, n)
				if len(result) >= limit {
					break
				}
			}
		}
		mutex.RUnlock()
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

// autoTagNote requests AI categorization for a note
func autoTagNote(noteID, userID, title, content string) {
	app.Log("notes", "Requesting tag generation for note: %s", noteID)

	text := content
	if title != "" {
		text = title + "\n\n" + content
	}

	data.Publish(data.Event{
		Type: data.EventGenerateTag,
		Data: map[string]interface{}{
			"note_id": noteID,
			"user_id": userID,
			"title":   title,
			"content": text,
			"type":    "note",
		},
	})
}

// addTagToNote adds a generated tag to an existing note
func addTagToNote(noteID, userID, tag string) {
	mutex.Lock()
	defer mutex.Unlock()

	for _, n := range notes {
		if n.ID == noteID && n.UserID == userID {
			// Don't overwrite if user already added tags
			if len(n.Tags) == 0 {
				n.Tags = []string{tag}
				n.UpdatedAt = time.Now()
				saveNotes()
				app.Log("notes", "Auto-tagged note %s with: %s", noteID, tag)
			}
			return
		}
	}
}

// indexNote indexes a note for RAG search
func indexNote(n *Note) {
	// Create unique ID per user+note
	indexID := fmt.Sprintf("note_%s_%s", n.UserID, n.ID)
	
	title := n.Title
	if title == "" {
		// Use first line of content as title
		lines := strings.SplitN(n.Content, "\n", 2)
		if len(lines[0]) > 50 {
			title = lines[0][:50] + "..."
		} else {
			title = lines[0]
		}
	}

	data.Index(
		indexID,
		"note",
		title,
		n.Content,
		map[string]interface{}{
			"user_id": n.UserID,
			"note_id": n.ID,
			"tags":    strings.Join(n.Tags, ","),
		},
	)
}

// reindexAllNotes indexes all notes (called on startup)
func reindexAllNotes() {
	mutex.RLock()
	notesCopy := make([]*Note, len(notes))
	copy(notesCopy, notes)
	mutex.RUnlock()

	for _, n := range notesCopy {
		indexNote(n)
	}
	app.Log("notes", "Indexed %d notes", len(notesCopy))
}

