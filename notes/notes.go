package notes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/data"
	"mu/tools"
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
	// Register tools
	RegisterNotesTools()

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

// RegisterNotesTools registers notes tools with the tools registry
func RegisterNotesTools() {
	tools.Register(tools.Tool{
		Name:        "notes.create",
		Description: "Create a new note for the user",
		Category:    "notes",
		Path:        "/api/notes",
		Method:      "POST",
		Input: map[string]tools.Param{
			"content": {Type: "string", Description: "Note content", Required: true},
			"title":   {Type: "string", Description: "Note title", Required: false},
			"tags":    {Type: "string", Description: "Comma-separated tags", Required: false},
		},
		Output: map[string]tools.Param{
			"id":    {Type: "string", Description: "Note ID"},
			"title": {Type: "string", Description: "Note title"},
		},
		Handler: handleNotesCreate,
	})

	tools.Register(tools.Tool{
		Name:        "notes.list",
		Description: "List user's notes",
		Category:    "notes",
		Path:        "/api/notes",
		Method:      "GET",
		Input: map[string]tools.Param{
			"tag":   {Type: "string", Description: "Filter by tag", Required: false},
			"limit": {Type: "number", Description: "Max results (default 10)", Required: false},
		},
		Output: map[string]tools.Param{
			"notes": {Type: "array", Description: "List of notes"},
		},
		Handler: handleNotesList,
	})

	tools.Register(tools.Tool{
		Name:        "notes.search",
		Description: "Search notes by keyword",
		Category:    "notes",
		Path:        "/api/notes/search",
		Method:      "GET",
		Input: map[string]tools.Param{
			"query": {Type: "string", Description: "Search query", Required: true},
		},
		Output: map[string]tools.Param{
			"results": {Type: "array", Description: "Matching notes"},
		},
		Handler: handleNotesSearch,
	})

	tools.Register(tools.Tool{
		Name:        "notes.get",
		Description: "Get a specific note by ID",
		Category:    "notes",
		Path:        "/api/notes/:id",
		Method:      "GET",
		Input: map[string]tools.Param{
			"id": {Type: "string", Description: "Note ID", Required: true},
		},
		Output: map[string]tools.Param{
			"id":      {Type: "string", Description: "Note ID"},
			"title":   {Type: "string", Description: "Note title"},
			"content": {Type: "string", Description: "Note content"},
			"tags":    {Type: "array", Description: "Note tags"},
		},
		Handler: handleNotesGet,
	})
}

func handleNotesCreate(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user not authenticated")
	}

	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	title, _ := params["title"].(string)
	tagsStr, _ := params["tags"].(string)

	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	note, err := CreateNote(userID, title, content, tags)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"id":    note.ID,
		"title": note.Title,
	}, nil
}

func handleNotesList(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user not authenticated")
	}

	tag, _ := params["tag"].(string)
	limit := 10
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	notesList := ListNotes(userID, false, tag, limit)

	var results []map[string]any
	for _, n := range notesList {
		results = append(results, map[string]any{
			"id":      n.ID,
			"title":   n.Title,
			"content": truncateNoteContent(n.Content, 100),
			"pinned":  n.Pinned,
		})
	}

	return map[string]any{"notes": results}, nil
}

func handleNotesSearch(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user not authenticated")
	}

	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	notesList := SearchNotes(userID, query, 5)

	var results []map[string]any
	for _, n := range notesList {
		results = append(results, map[string]any{
			"id":      n.ID,
			"title":   n.Title,
			"content": truncateNoteContent(n.Content, 100),
		})
	}

	return map[string]any{"results": results}, nil
}

func handleNotesGet(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user not authenticated")
	}

	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	note := GetNote(id, userID)
	if note == nil {
		return nil, fmt.Errorf("note not found")
	}

	return map[string]any{
		"id":      note.ID,
		"title":   note.Title,
		"content": note.Content,
		"tags":    note.Tags,
	}, nil
}

func truncateNoteContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
