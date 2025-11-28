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
	os.MkdirAll(path, 0700)
	os.WriteFile(file, b, 0644)

	return nil
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
	Keywords  []string               `json:"keywords"`
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
	// Check if custom keywords are provided in metadata
	var keywords []string
	if customKeywords, ok := metadata["keywords"]; ok {
		if keywordSlice, ok := customKeywords.([]string); ok {
			keywords = keywordSlice
		}
	}
	
	// If no custom keywords, extract from title and content
	if len(keywords) == 0 {
		keywords = extractKeywords(title + " " + content)
	}

	entry := &IndexEntry{
		ID:        id,
		Type:      entryType,
		Title:     title,
		Content:   content,
		Metadata:  metadata,
		Keywords:  keywords,
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
func Search(query string, limit int) []*IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	// Try vector search first
	queryEmbedding, err := getEmbedding(query)
	if err == nil && len(queryEmbedding) > 0 {
		fmt.Printf("[SEARCH] Using vector search for: %s\n", query)
		var results []SearchResult

		for _, entry := range index {
			if len(entry.Embedding) == 0 {
				continue // Skip entries without embeddings
			}
			
			similarity := cosineSimilarity(queryEmbedding, entry.Embedding)
			if similarity > 0.3 { // Threshold to filter irrelevant results
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

		// Return top N results
		if limit > 0 && len(results) > limit {
			results = results[:limit]
		}

		entries := make([]*IndexEntry, len(results))
		for i, r := range results {
			entries[i] = r.Entry
		}

		if len(entries) > 0 {
			return entries
		}
	}

	// Fallback to keyword search if vector search fails or returns no results
	fmt.Printf("[SEARCH] Using keyword fallback for: %s\n", query)
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		fmt.Printf("[SEARCH] No keywords extracted from query\n")
		return nil
	}
	fmt.Printf("[SEARCH] Extracted keywords: %v\n", keywords)

	var results []SearchResult

	for _, entry := range index {
		score := calculateRelevance(keywords, entry)
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

// extractKeywords performs basic keyword extraction
func extractKeywords(text string) []string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Remove punctuation and split into words
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-')
	})

	// Filter stop words and short words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "as": true, "is": true, "was": true,
		"are": true, "were": true, "been": true, "be": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"this": true, "that": true, "these": true, "those": true, "i": true, "you": true,
		"he": true, "she": true, "it": true, "we": true, "they": true, "what": true,
		"which": true, "who": true, "when": true, "where": true, "why": true, "how": true,
	}

	var keywords []string
	seen := make(map[string]bool)

	for _, word := range words {
		if len(word) < 3 || stopWords[word] || seen[word] {
			continue
		}
		keywords = append(keywords, word)
		seen[word] = true
	}

	return keywords
}

// calculateRelevance scores an entry against query keywords
func calculateRelevance(queryKeywords []string, entry *IndexEntry) float64 {
	if len(queryKeywords) == 0 {
		return 0
	}

	score := 0.0
	titleLower := strings.ToLower(entry.Title)
	contentLower := strings.ToLower(entry.Content)

	for _, keyword := range queryKeywords {
		// Title matches are worth more
		if strings.Contains(titleLower, keyword) {
			score += 3.0
		}

		// Content matches
		if strings.Contains(contentLower, keyword) {
			score += 1.0
		}

		// Keyword matches (pre-extracted)
		for _, entryKeyword := range entry.Keywords {
			if entryKeyword == keyword {
				score += 2.0
				break
			}
		}
	}

	return score
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
