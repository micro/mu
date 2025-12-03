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
	EventRefreshHNComments = "refresh_hn_comments"
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

// EventHandler is a function that handles events
type EventHandler func(Event)

var (
	eventMutex    sync.RWMutex
	eventHandlers = make(map[string][]EventHandler)
)

// Subscribe registers a handler for a specific event type
func Subscribe(eventType string, handler EventHandler) {
	eventMutex.Lock()
	defer eventMutex.Unlock()
	eventHandlers[eventType] = append(eventHandlers[eventType], handler)
}

// Publish sends an event to all registered handlers
func Publish(event Event) {
	eventMutex.RLock()
	handlers := eventHandlers[event.Type]
	eventMutex.RUnlock()

	for _, handler := range handlers {
		go handler(event) // Run handlers asynchronously
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

var (
	indexMutex sync.RWMutex
	index      = make(map[string]*IndexEntry)
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

// Index adds or updates an entry in the search index
func Index(id, entryType, title, content string, metadata map[string]interface{}) {
	// Check if already indexed (skip if exists and recent)
	indexMutex.RLock()
	if existing, exists := index[id]; exists {
		// If indexed within last 30 seconds, skip re-indexing
		if time.Since(existing.IndexedAt) < 30*time.Second {
			indexMutex.RUnlock()
			fmt.Printf("[data] Skipping re-index of %s (indexed %v ago)\n", id, time.Since(existing.IndexedAt).Round(time.Second))
			return
		}
	}
	indexMutex.RUnlock()

	fmt.Printf("[data] Indexing %s: %s\n", entryType, title)

	entry := &IndexEntry{
		ID:        id,
		Type:      entryType,
		Title:     title,
		Content:   content,
		Metadata:  metadata,
		IndexedAt: time.Now(),
	}

	// Generate embedding for semantic search
	textToEmbed := title
	if len(content) > 0 {
		// Combine title and beginning of content for better embeddings
		maxContent := 500
		if len(content) < maxContent {
			maxContent = len(content)
		}
		textToEmbed = title + " " + content[:maxContent]
	}

	embedding, err := getEmbedding(textToEmbed)
	if err == nil && len(embedding) > 0 {
		entry.Embedding = embedding
	}

	indexMutex.Lock()
	index[id] = entry
	indexMutex.Unlock()

	// Persist to disk
	saveIndex()
}

// GetByID retrieves an entry by its exact ID
func GetByID(id string) *IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()
	return index[id]
}

// Search performs semantic vector search with keyword fallback
func Search(query string, limit int, opts ...SearchOption) []*IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	// Apply options
	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Try vector search first
	queryEmbedding, err := getEmbedding(query)
	if err == nil && len(queryEmbedding) > 0 {
		fmt.Printf("[SEARCH] Using vector search for: %s (type: %s)\n", query, options.Type)

		// Convert map to slice for parallel processing, optionally filtering by type
		entries := make([]*IndexEntry, 0, len(index))
		for _, entry := range index {
			if len(entry.Embedding) > 0 && (options.Type == "" || entry.Type == options.Type) {
				entries = append(entries, entry)
			}
		}

		// Parallel search using goroutines
		numWorkers := 4
		chunkSize := (len(entries) + numWorkers - 1) / numWorkers
		resultsChan := make(chan []SearchResult, numWorkers)

		for i := 0; i < numWorkers; i++ {
			start := i * chunkSize
			end := start + chunkSize
			if end > len(entries) {
				end = len(entries)
			}
			if start >= len(entries) {
				break
			}

			go func(chunk []*IndexEntry) {
				var localResults []SearchResult
				for _, entry := range chunk {
					similarity := cosineSimilarity(queryEmbedding, entry.Embedding)
					if similarity > 0.3 { // Threshold to filter irrelevant results
						localResults = append(localResults, SearchResult{
							Entry: entry,
							Score: similarity,
						})
					}
				}
				resultsChan <- localResults
			}(entries[start:end])
		}

		// Collect results from all workers
		var results []SearchResult
		for i := 0; i < numWorkers && i*chunkSize < len(entries); i++ {
			results = append(results, <-resultsChan...)
		}
		close(resultsChan)

		// Sort by similarity descending
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		// Return top N results
		if limit > 0 && len(results) > limit {
			results = results[:limit]
		}

		finalEntries := make([]*IndexEntry, len(results))
		for i, r := range results {
			finalEntries[i] = r.Entry
		}

		if len(finalEntries) > 0 {
			return finalEntries
		}
	}

	// Fallback: Simple text matching if vector search fails or returns no results
	fmt.Printf("[SEARCH] Using text fallback for: %s (type: %s)\n", query, options.Type)
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
	indexMutex.Unlock()
	saveIndex()
}

// saveIndex persists the index to disk
func saveIndex() {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	SaveJSON("index.json", index)
}

// Load loads the index from disk
func Load() {
	b, err := LoadFile("index.json")
	if err != nil {
		return
	}

	indexMutex.Lock()
	defer indexMutex.Unlock()

	json.Unmarshal(b, &index)
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
