package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================
// EVENT SYSTEM
// ============================================

// Event types
const (
	EventRefreshHNComments  = "refresh_hn_comments"
	EventIndexComplete      = "index_complete"
	EventNewArticleMetadata = "new_article_metadata"
	EventGenerateSummary    = "generate_summary"
	EventSummaryGenerated   = "summary_generated"
	EventGenerateTag        = "generate_tag"
	EventTagGenerated       = "tag_generated"
)

// Event represents a data event
type Event struct {
	Type string
	Data map[string]interface{}
}

// SearchOptions configures search behavior
type SearchOptions struct {
	Type string
}

// SearchOption is a functional option for configuring search
type SearchOption func(*SearchOptions)

// WithType filters search results by entry type
func WithType(entryType string) SearchOption {
	return func(opts *SearchOptions) {
		opts.Type = entryType
	}
}

// EventSubscription represents an active subscription
type EventSubscription struct {
	Chan      chan Event
	eventType string
	id        string
}

var (
	eventMutex       sync.RWMutex
	eventSubscribers = make(map[string]map[string]chan Event) // eventType -> subscriberID -> channel
	subscriberIDSeq  int
)

// Subscribe creates a channel-based subscription for a specific event type
func Subscribe(eventType string) *EventSubscription {
	eventMutex.Lock()
	defer eventMutex.Unlock()

	// Generate unique subscriber ID
	subscriberIDSeq++
	id := fmt.Sprintf("sub_%d", subscriberIDSeq)

	// Create buffered channel to prevent blocking
	ch := make(chan Event, 10)

	// Initialize map if needed
	if eventSubscribers[eventType] == nil {
		eventSubscribers[eventType] = make(map[string]chan Event)
	}

	eventSubscribers[eventType][id] = ch

	return &EventSubscription{
		Chan:      ch,
		eventType: eventType,
		id:        id,
	}
}

// Close closes the channel and removes the subscription
func (s *EventSubscription) Close() {
	eventMutex.Lock()
	defer eventMutex.Unlock()

	if subscribers, ok := eventSubscribers[s.eventType]; ok {
		if ch, ok := subscribers[s.id]; ok {
			close(ch)
			delete(subscribers, s.id)
		}
	}
}

// Publish sends an event to all subscribers
func Publish(event Event) {
	eventMutex.RLock()
	subscribers := eventSubscribers[event.Type]
	eventMutex.RUnlock()

	// Send to channel subscribers (non-blocking)
	for _, ch := range subscribers {
		select {
		case ch <- event:
			// Sent successfully
		default:
			// Channel full, skip (subscriber should have buffer or be reading)
		}
	}
}

// SaveFile saves data to disk
func SaveFile(key, val string) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	os.MkdirAll(path, 0700)
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
	indexMutex          sync.RWMutex
	index               = make(map[string]*IndexEntry)
	embeddings          = make(map[string][]float64) // Stored separately from index
	savePending         = false
	saveMutex           sync.Mutex
	embeddingCache      = make(map[string][]float64) // Cache query embeddings
	embeddingCacheMu    sync.RWMutex
	maxEmbeddingCache   = 100   // Maximum cached query embeddings
	maxIndexEmbeddings  = 10000 // Maximum index entries with embeddings
	embeddingQueue      = make(chan string, 100)
	embeddingEnabled    = false
	embeddingMutex      sync.Mutex
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
	Embedding []float64              `json:"embedding"` // Vector embedding for semantic search
	IndexedAt time.Time              `json:"indexed_at"`
}

// SearchResult represents a search hit with relevance score
type SearchResult struct {
	Entry *IndexEntry
	Score float64
}

// Index queues an entry to be added or updated in the search index
func Index(id, entryType, title, content string, metadata map[string]interface{}) {
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

	// Preserve existing embedding if content hasn't changed
	if exists && existing.Title == work.Title && existing.Content == work.Content {
		indexMutex.RLock()
		if emb, hasEmb := embeddings[work.ID]; hasEmb && len(emb) > 0 {
			// Keep the existing embedding
			indexMutex.RUnlock()
		} else {
			indexMutex.RUnlock()
		}
	}

	indexMutex.Lock()
	index[work.ID] = entry
	indexMutex.Unlock()

	// Publish event that indexing is complete
	Publish(Event{
		Type: EventIndexComplete,
		Data: map[string]interface{}{
			"id":   work.ID,
			"type": work.Type,
		},
	})

	// Only queue for embedding if we don't already have one
	indexMutex.RLock()
	_, hasEmbedding := embeddings[work.ID]
	indexMutex.RUnlock()

	if !hasEmbedding {
		select {
		case embeddingQueue <- work.ID:
		default:
			// Queue full, skip
		}
	}

	// Persist to disk (debounced)
	go saveIndex()
}

// StartIndexing enables background embedding generation and index workers
func StartIndexing() {
	embeddingMutex.Lock()
	if embeddingEnabled {
		embeddingMutex.Unlock()
		return
	}
	embeddingEnabled = true
	embeddingMutex.Unlock()

	fmt.Println("[data] Starting background embedding worker")
	go embeddingWorker()

	// Start index workers if not already started
	if !indexWorkersStarted {
		indexWorkersStarted = true
		numWorkers := 4 // Use 4 workers to process index queue
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

// embeddingWorker processes the embedding queue with rate limiting
func embeddingWorker() {
	for id := range embeddingQueue {
		// Check if we've hit the embedding limit
		indexMutex.RLock()
		embeddingCount := len(embeddings)
		entry := index[id]
		_, hasEmbedding := embeddings[id]
		indexMutex.RUnlock()

		if embeddingCount >= maxIndexEmbeddings {
			fmt.Printf("[data] Hit embedding limit (%d), skipping new embeddings\n", maxIndexEmbeddings)
			continue
		}

		if entry == nil || hasEmbedding {
			continue
		}

		// Generate embedding
		textToEmbed := entry.Title
		if len(entry.Content) > 0 {
			maxContent := 500
			if len(entry.Content) < maxContent {
				maxContent = len(entry.Content)
			}
			textToEmbed = entry.Title + " " + entry.Content[:maxContent]
		}

		embedding, err := getEmbedding(textToEmbed)
		if err != nil {
			fmt.Printf("[data] Failed to generate embedding for %s: %v\n", id, err)
			time.Sleep(1 * time.Second) // Back off on error
			continue
		}

		if len(embedding) > 0 {
			indexMutex.Lock()
			if index[id] != nil {
				embeddings[id] = embedding
			}
			indexMutex.Unlock()
			go saveIndex()
			go saveEmbeddings()
		}

		// Rate limit: 1 embedding per 200ms to avoid overwhelming Ollama
		time.Sleep(200 * time.Millisecond)
	}
}

// GetByID retrieves an entry by its exact ID
func GetByID(id string) *IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()
	return index[id]
}

// Search performs semantic vector search with text fallback
func Search(query string, limit int, opts ...SearchOption) []*IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	// Apply options
	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Check if we have any embeddings
	hasEmbeddings := len(embeddings) > 0

	// Try vector search if embeddings exist
	if hasEmbeddings {
		// Check cache first
		embeddingCacheMu.RLock()
		queryEmbedding, cached := embeddingCache[query]
		embeddingCacheMu.RUnlock()

		// Generate embedding if not cached
		if !cached {
			var err error
			queryEmbedding, err = getEmbedding(query)
			if err == nil && len(queryEmbedding) > 0 {
				// Cache it with proper limit
				embeddingCacheMu.Lock()
				// Remove oldest entry if at limit
				if len(embeddingCache) >= maxEmbeddingCache {
					// Just clear a random entry (simple approach)
					for k := range embeddingCache {
						delete(embeddingCache, k)
						break
					}
				}
				embeddingCache[query] = queryEmbedding
				embeddingCacheMu.Unlock()
			}
		}

		if len(queryEmbedding) > 0 {
			fmt.Printf("[SEARCH] Using vector search for: %s (type: %s)\n", query, options.Type)

			// Simple linear search through entries with embeddings
			var results []SearchResult
			for _, entry := range index {
				emb, hasEmb := embeddings[entry.ID]
				if !hasEmb || len(emb) == 0 || (options.Type != "" && entry.Type != options.Type) {
					continue
				}

				similarity := cosineSimilarity(queryEmbedding, emb)
				if similarity > 0.3 {
					results = append(results, SearchResult{
						Entry: entry,
						Score: similarity,
					})
				}
			}

			// Sort by similarity descending
			sort.Slice(results, func(i, j int) bool {
				return results[i].Score > results[j].Score
			})

			fmt.Printf("[SEARCH] Vector search found %d results\n", len(results))

			// Always also do text search to catch exact keyword matches
			// that semantic search might miss
			queryLower := strings.ToLower(query)
			textResults := make(map[string]float64) // ID -> score

			for _, entry := range index {
				// Filter by type if specified
				if options.Type != "" && entry.Type != options.Type {
					continue
				}

				score := 0.0
				titleLower := strings.ToLower(entry.Title)
				contentLower := strings.ToLower(entry.Content)

				// Simple contains matching with higher weight for title matches
				if strings.Contains(titleLower, queryLower) {
					score = 5.0 // Boosted title match score to rank above low semantic scores
				} else if strings.Contains(contentLower, queryLower) {
					score = 2.0 // Boosted content match score
				}

				if score > 0 {
					textResults[entry.ID] = score
				}
			}

			fmt.Printf("[SEARCH] Text search found %d additional exact matches\n", len(textResults))

			// Merge vector and text results, preferring text matches for exact keywords
			mergedResults := make(map[string]SearchResult)
			for _, r := range results {
				mergedResults[r.Entry.ID] = r
			}

			// Add or boost text search results
			for id, textScore := range textResults {
				if existing, found := mergedResults[id]; found {
					// Entry found in both - boost the score if text match is better
					if textScore > existing.Score {
						existing.Score = textScore
						mergedResults[id] = existing
					}
				} else {
					// New entry from text search only
					if entry := index[id]; entry != nil {
						mergedResults[id] = SearchResult{
							Entry: entry,
							Score: textScore,
						}
					}
				}
			}

			// Convert back to slice and sort
			var finalResults []SearchResult
			for _, r := range mergedResults {
				finalResults = append(finalResults, r)
			}

			sort.Slice(finalResults, func(i, j int) bool {
				return finalResults[i].Score > finalResults[j].Score
			})

			// Return top N results
			if limit > 0 && len(finalResults) > limit {
				finalResults = finalResults[:limit]
			}

			if len(finalResults) > 0 {
				entries := make([]*IndexEntry, len(finalResults))
				for i, r := range finalResults {
					entries[i] = r.Entry
				}
				return entries
			}
		}
	}

	// Fallback to pure text search if no embeddings available
	fmt.Printf("[SEARCH] Using pure text search for: %s (type: %s)\n", query, options.Type)
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
	embeddings = make(map[string][]float64)
	indexMutex.Unlock()
	saveIndex()
	saveEmbeddings()
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

var (
	embSavePending = false
	embSaveMutex   sync.Mutex
)

// saveEmbeddings persists embeddings to disk separately
func saveEmbeddings() {
	// Debounce saves
	embSaveMutex.Lock()
	if embSavePending {
		embSaveMutex.Unlock()
		return
	}
	embSavePending = true
	embSaveMutex.Unlock()

	// Wait a bit to batch multiple updates
	time.Sleep(1 * time.Second)

	indexMutex.RLock()
	SaveJSON("embeddings.json", embeddings)
	indexMutex.RUnlock()

	embSaveMutex.Lock()
	embSavePending = false
	embSaveMutex.Unlock()
}

// Load loads the index and embeddings from disk
func Load() {
	// Load index (may contain old format with embeddings inline)
	b, err := LoadFile("index.json")
	if err == nil {
		indexMutex.Lock()
		// First try to load as old format (with embeddings)
		var oldIndex map[string]*struct {
			ID        string                 `json:"id"`
			Type      string                 `json:"type"`
			Title     string                 `json:"title"`
			Content   string                 `json:"content"`
			Metadata  map[string]interface{} `json:"metadata,omitempty"`
			IndexedAt time.Time              `json:"indexed_at"`
			Embedding []float64              `json:"embedding,omitempty"`
		}

		if err := json.Unmarshal(b, &oldIndex); err == nil {
			// Migrate old format to new format
			for id, old := range oldIndex {
				index[id] = &IndexEntry{
					ID:        old.ID,
					Type:      old.Type,
					Title:     old.Title,
					Content:   old.Content,
					Metadata:  old.Metadata,
					IndexedAt: old.IndexedAt,
				}
				// Extract embedding if present
				if len(old.Embedding) > 0 {
					embeddings[id] = old.Embedding
				}
			}
		}
		indexMutex.Unlock()
	}

	// Load embeddings (if new format exists, it will override migrated ones)
	b, err = LoadFile("embeddings.json")
	if err == nil {
		indexMutex.Lock()
		json.Unmarshal(b, &embeddings)
		indexMutex.Unlock()
	}
}

// ============================================
// VECTOR EMBEDDINGS VIA OLLAMA
// ============================================

// getEmbedding generates a vector embedding for text using Ollama
func getEmbedding(text string) ([]float64, error) {
	if len(text) == 0 {
		return nil, fmt.Errorf("empty text")
	}

	fmt.Printf("[data] Generating embedding for text (length: %d)\n", len(text))

	// Ollama embedding endpoint
	url := "http://localhost:11434/api/embeddings"

	requestBody := map[string]interface{}{
		"model":  "nomic-embed-text",
		"prompt": text,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error: %s", string(body))
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Embedding, nil
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
