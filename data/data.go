package data

import (
	"encoding/json"
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
	IndexedAt time.Time              `json:"indexed_at"`
}

// SearchResult represents a search hit with relevance score
type SearchResult struct {
	Entry *IndexEntry
	Score float64
}

// Index adds or updates an entry in the search index
func Index(id, entryType, title, content string, metadata map[string]interface{}) {
	keywords := extractKeywords(title + " " + content)

	entry := &IndexEntry{
		ID:        id,
		Type:      entryType,
		Title:     title,
		Content:   content,
		Metadata:  metadata,
		Keywords:  keywords,
		IndexedAt: time.Now(),
	}

	indexMutex.Lock()
	index[id] = entry
	indexMutex.Unlock()

	// Persist to disk
	saveIndex()
}

// Search performs keyword-based search and returns ranked results
func Search(query string, limit int) []*IndexEntry {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil
	}

	indexMutex.RLock()
	defer indexMutex.RUnlock()

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
