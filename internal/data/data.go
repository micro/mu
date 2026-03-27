package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/event"
)

// SearchOptions configures search behavior
type SearchOptions struct {
	Type        string
	KeywordOnly bool // Use keyword matching only
}

// SearchOption is a functional option for configuring search
type SearchOption func(*SearchOptions)

// WithType filters search results by entry type
func WithType(entryType string) SearchOption {
	return func(opts *SearchOptions) {
		opts.Type = entryType
	}
}

// WithKeywordOnly uses keyword matching only
func WithKeywordOnly() SearchOption {
	return func(opts *SearchOptions) {
		opts.KeywordOnly = true
	}
}

// SaveFile saves data to disk
func SaveFile(key, val string) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	// Create all parent directories including subdirectories in key
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, []byte(val), 0644)
	return nil
}

// LoadFile loads a file from disk
func LoadFile(key string) ([]byte, error) {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	return os.ReadFile(file)
}

func DeleteFile(key string) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	return os.Remove(file)
}

func SaveJSON(key string, val interface{}) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}

	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)

	// Create all parent directories
	fileDir := filepath.Dir(file)
	os.MkdirAll(fileDir, 0700)

	os.WriteFile(file, b, 0644)

	return nil
}

func LoadJSON(key string, val interface{}) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)

	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, val)
}

// ============================================
// ADMIN DELETE REGISTRY
// ============================================

// DeleteFunc deletes an item by ID. Returns error if not found or failed.
type DeleteFunc func(id string) error

var (
	deleterMu sync.RWMutex
	deleters  = map[string]DeleteFunc{}
)

// RegisterDeleter registers a delete function for a content type.
// Packages call this during Load() so admin can delete any content by type+ID.
func RegisterDeleter(contentType string, fn DeleteFunc) {
	deleterMu.Lock()
	deleters[contentType] = fn
	deleterMu.Unlock()
}

// Delete deletes an item by type and ID using the registered deleter.
func Delete(contentType, id string) error {
	deleterMu.RLock()
	fn, ok := deleters[contentType]
	deleterMu.RUnlock()
	if !ok {
		return fmt.Errorf("no deleter registered for type %q", contentType)
	}
	return fn(id)
}

// DeleteTypes returns all registered content types that support deletion.
func DeleteTypes() []string {
	deleterMu.RLock()
	defer deleterMu.RUnlock()
	types := make([]string, 0, len(deleters))
	for t := range deleters {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// ============================================
// SIMPLE INDEXING & SEARCH FOR RAG
// ============================================

// IndexWork represents a work item for the indexing queue
type IndexWork struct {
	ID       string
	Type     string
	Title    string
	Content  string
	Metadata map[string]interface{}
}

var (
	// UseSQLite enables SQLite backend instead of in-memory maps
	// Set via MU_USE_SQLITE=1 environment variable
	UseSQLite = os.Getenv("MU_USE_SQLITE") == "1"

	indexMutex          sync.RWMutex
	index               = make(map[string]*IndexEntry)
	savePending         = false
	saveMutex           sync.Mutex
	indexWorkQueue      = make(chan IndexWork, 500) // Buffer up to 500 pending index operations
	indexWorkersStarted = false
)

// IndexEntry represents a searchable piece of content
type IndexEntry struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // "news", "video", "market", "reminder"
	Title     string                 `json:"title"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	IndexedAt time.Time              `json:"indexed_at"`
}

// SearchResult represents a search hit with relevance score
type SearchResult struct {
	Entry *IndexEntry
	Score float64
}

// Index queues an entry to be added or updated in the search index
func Index(id, entryType, title, content string, metadata map[string]interface{}) {
	// Use SQLite backend if enabled
	if UseSQLite {
		if err := IndexSQLite(id, entryType, title, content, metadata); err != nil {
			fmt.Printf("[data] SQLite index error: %v\n", err)
		}
		return
	}

	// Queue the work instead of processing immediately
	select {
	case indexWorkQueue <- IndexWork{
		ID:       id,
		Type:     entryType,
		Title:    title,
		Content:  content,
		Metadata: metadata,
	}:
		// Work queued successfully
	default:
		// Queue full, process synchronously to avoid dropping
		processIndexWork(IndexWork{
			ID:       id,
			Type:     entryType,
			Title:    title,
			Content:  content,
			Metadata: metadata,
		})
	}
}

// processIndexWork does the actual indexing work
func processIndexWork(work IndexWork) {
	indexMutex.RLock()
	existing, exists := index[work.ID]
	indexMutex.RUnlock()

	// Skip if already exists with same title/content
	if exists {
		contentSame := existing.Title == work.Title && existing.Content == work.Content

		// If content is the same, skip entirely (no need to re-index)
		if contentSame {
			// Still update metadata if it changed (e.g., new comments)
			if work.Metadata != nil {
				metadataChanged := false
				for k, v := range work.Metadata {
					if existingVal, ok := existing.Metadata[k]; !ok || existingVal != v {
						metadataChanged = true
						break
					}
				}
				if metadataChanged {
					indexMutex.Lock()
					existing.Metadata = work.Metadata
					indexMutex.Unlock()
					go saveIndex()
				}
			}
			return
		}

		// Content changed, allow re-index
	}

	entry := &IndexEntry{
		ID:        work.ID,
		Type:      work.Type,
		Title:     work.Title,
		Content:   work.Content,
		Metadata:  work.Metadata,
		IndexedAt: time.Now(),
	}

	indexMutex.Lock()
	index[work.ID] = entry
	indexMutex.Unlock()

	// Publish event that indexing is complete
	event.Publish(event.Event{
		Type: event.EventIndexComplete,
		Data: map[string]interface{}{
			"id":   work.ID,
			"type": work.Type,
		},
	})

	// Persist to disk (debounced)
	go saveIndex()
}

// StartIndexing enables background index workers
func StartIndexing() {
	if !indexWorkersStarted {
		indexWorkersStarted = true
		numWorkers := 4
		fmt.Printf("[data] Starting %d index workers\n", numWorkers)
		for i := 0; i < numWorkers; i++ {
			go indexWorker(i)
		}
	}
}

// indexWorker processes items from the index work queue
func indexWorker(id int) {
	for work := range indexWorkQueue {
		processIndexWork(work)
	}
}


// GetByID retrieves an entry by its exact ID
func GetByID(id string) *IndexEntry {
	if UseSQLite {
		entry, err := GetByIDSQLite(id)
		if err != nil {
			fmt.Printf("[data] SQLite GetByID error: %v\n", err)
			return nil
		}
		return entry
	}

	indexMutex.RLock()
	defer indexMutex.RUnlock()
	return index[id]
}

// Search performs full-text search across indexed content
func Search(query string, limit int, opts ...SearchOption) []*IndexEntry {
	if UseSQLite {
		results, err := SearchSQLite(query, limit, opts...)
		if err != nil {
			fmt.Printf("[data] SQLite Search error: %v\n", err)
			return nil
		}
		return results
	}

	indexMutex.RLock()
	defer indexMutex.RUnlock()

	// Apply options
	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Text search
	queryLower := strings.ToLower(query)
	var results []SearchResult

	for _, entry := range index {
		// Filter by type if specified
		if options.Type != "" && entry.Type != options.Type {
			continue
		}

		score := 0.0
		titleLower := strings.ToLower(entry.Title)
		contentLower := strings.ToLower(entry.Content)

		// Simple contains matching
		if strings.Contains(titleLower, queryLower) {
			score = 3.0
		} else if strings.Contains(contentLower, queryLower) {
			score = 1.0
		}

		if score > 0 {
			results = append(results, SearchResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return top N results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	entries := make([]*IndexEntry, len(results))
	for i, r := range results {
		entries[i] = r.Entry
	}

	return entries
}

// GetByType returns all entries of a specific type
func GetByType(entryType string, limit int) []*IndexEntry {
	if UseSQLite {
		results, err := GetByTypeSQLite(entryType, limit)
		if err != nil {
			fmt.Printf("[data] SQLite GetByType error: %v\n", err)
			return nil
		}
		return results
	}

	indexMutex.RLock()
	defer indexMutex.RUnlock()

	var entries []*IndexEntry
	for _, entry := range index {
		if entry.Type == entryType {
			entries = append(entries, entry)
		}
	}

	// Sort by indexed time descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].IndexedAt.After(entries[j].IndexedAt)
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries
}

// ClearIndex removes all entries from the index
func ClearIndex() {
	indexMutex.Lock()
	index = make(map[string]*IndexEntry)
	indexMutex.Unlock()
	saveIndex()
}

// saveIndex persists the index to disk
func saveIndex() {
	// Debounce saves - only save once even if called multiple times
	saveMutex.Lock()
	if savePending {
		saveMutex.Unlock()
		return
	}
	savePending = true
	saveMutex.Unlock()

	// Wait a bit to batch multiple index updates
	time.Sleep(1 * time.Second)

	indexMutex.RLock()
	SaveJSON("index.json", index)
	indexMutex.RUnlock()

	saveMutex.Lock()
	savePending = false
	saveMutex.Unlock()
}

// Load loads the index from disk
func Load() {
	// If SQLite is enabled, migrate from JSON and use SQLite
	if UseSQLite {
		fmt.Println("[data] SQLite backend enabled")
		if err := MigrateFromJSON(); err != nil {
			fmt.Printf("[data] Migration error: %v\n", err)
		}
		EnsureFTS()
		// Get stats
		entries, embCount, err := GetIndexStats()
		if err == nil {
			fmt.Printf("[data] SQLite index: %d entries, %d embeddings\n", entries, embCount)
		}
		return
	}

	// Legacy in-memory loading
	b, err := LoadFile("index.json")
	if err == nil {
		indexMutex.Lock()
		json.Unmarshal(b, &index)
		indexMutex.Unlock()
		fmt.Printf("[data] Loaded %d index entries from disk\n", len(index))
	}
}

// Stats holds index statistics
type Stats struct {
	TotalEntries int  `json:"total_entries"`
	UsingSQLite  bool `json:"using_sqlite"`
}

// GetStats returns current index statistics
func GetStats() Stats {
	if UseSQLite {
		entries, _, _ := GetIndexStats()
		return Stats{
			TotalEntries: entries,
			UsingSQLite:  true,
		}
	}

	indexMutex.RLock()
	entryCount := len(index)
	indexMutex.RUnlock()

	return Stats{
		TotalEntries: entryCount,
	}
}
