package data

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSQLiteMigration(t *testing.T) {
	// Use a temp directory for test
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Reset singleton
	dbOnce = sync.Once{}
	db = nil

	// Create test index.json
	testIndex := `{
		"test1": {
			"id": "test1",
			"type": "news",
			"title": "Test Article",
			"content": "This is test content about technology and AI.",
			"metadata": {"url": "https://example.com/test1"},
			"indexed_at": "2024-01-01T00:00:00Z"
		},
		"test2": {
			"id": "test2",
			"type": "video",
			"title": "Test Video",
			"content": "Video about machine learning.",
			"metadata": {"channel": "TestChannel"},
			"indexed_at": "2024-01-02T00:00:00Z"
		}
	}`

	dataDir := filepath.Join(tempDir, ".mu", "data")
	os.MkdirAll(dataDir, 0755)
	os.WriteFile(filepath.Join(dataDir, "index.json"), []byte(testIndex), 0644)

	// Run migration
	err := MigrateFromJSON()
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify stats
	entries, _, err := GetIndexStats()
	if err != nil {
		t.Fatalf("GetIndexStats failed: %v", err)
	}
	if entries != 2 {
		t.Errorf("Expected 2 entries, got %d", entries)
	}

	// Test GetByID
	entry, err := GetByIDSQLite("test1")
	if err != nil {
		t.Fatalf("GetByIDSQLite failed: %v", err)
	}
	if entry == nil {
		t.Fatal("Entry not found")
	}
	if entry.Title != "Test Article" {
		t.Errorf("Expected 'Test Article', got '%s'", entry.Title)
	}
	if entry.Metadata["url"] != "https://example.com/test1" {
		t.Errorf("Metadata not preserved")
	}

	// Test GetByType
	newsEntries, err := GetByTypeSQLite("news", 10)
	if err != nil {
		t.Fatalf("GetByTypeSQLite failed: %v", err)
	}
	if len(newsEntries) != 1 {
		t.Errorf("Expected 1 news entry, got %d", len(newsEntries))
	}
}

func TestSQLiteSearch(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	dbOnce = sync.Once{}
	db = nil

	// Initialize and add some entries
	err := IndexSQLite("search1", "news", "AI Revolution in Tech", "Artificial intelligence is transforming the technology industry.", nil)
	if err != nil {
		t.Fatalf("IndexSQLite failed: %v", err)
	}

	err = IndexSQLite("search2", "news", "Sports Update", "Football season kicks off with exciting matches.", nil)
	if err != nil {
		t.Fatalf("IndexSQLite failed: %v", err)
	}

	err = IndexSQLite("search3", "video", "Machine Learning Tutorial", "Learn about neural networks and deep learning.", nil)
	if err != nil {
		t.Fatalf("IndexSQLite failed: %v", err)
	}

	// Test search - use a term that's actually in the content
	results, err := SearchSQLite("intelligence", 10)
	if err != nil {
		t.Fatalf("SearchSQLite failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected search results for 'intelligence'")
	}

	// Test search by title
	results, err = SearchSQLite("Revolution", 10)
	if err != nil {
		t.Fatalf("SearchSQLite failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'Revolution', got %d", len(results))
	}

	// Test search with type filter
	results, err = SearchSQLite("learning", 10, WithType("video"))
	if err != nil {
		t.Fatalf("SearchSQLite with type failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 video result, got %d", len(results))
	}
}

func TestSQLiteIndexUpdate(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	dbOnce = sync.Once{}
	db = nil

	// Add entry
	err := IndexSQLite("update1", "news", "Original Title", "Original content.", map[string]interface{}{"version": 1})
	if err != nil {
		t.Fatalf("IndexSQLite failed: %v", err)
	}

	// Update entry
	err = IndexSQLite("update1", "news", "Updated Title", "Updated content.", map[string]interface{}{"version": 2})
	if err != nil {
		t.Fatalf("IndexSQLite update failed: %v", err)
	}

	// Verify update
	entry, err := GetByIDSQLite("update1")
	if err != nil {
		t.Fatalf("GetByIDSQLite failed: %v", err)
	}
	if entry.Title != "Updated Title" {
		t.Errorf("Expected 'Updated Title', got '%s'", entry.Title)
	}
	if entry.Metadata["version"] != float64(2) {
		t.Errorf("Expected version 2, got %v", entry.Metadata["version"])
	}

	// Verify only one entry exists
	entries, _, _ := GetIndexStats()
	if entries != 1 {
		t.Errorf("Expected 1 entry after update, got %d", entries)
	}
}

func TestFTS5Search(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	dbOnce = sync.Once{}
	db = nil

	// Add entries
	IndexSQLite("fts1", "news", "Bitcoin Hits All-Time High", "Bitcoin reached a new record price amid growing institutional adoption.", nil)
	IndexSQLite("fts2", "news", "Ethereum Upgrade Complete", "The Ethereum network completed its latest upgrade successfully.", nil)
	IndexSQLite("fts3", "news", "Sports News Today", "Football league results and highlights from this weekend.", nil)

	// FTS5 should find this via full-text search
	results, err := SearchSQLite("bitcoin institutional", 10)
	if err != nil {
		t.Fatalf("FTS search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected FTS results for 'bitcoin institutional'")
	}
	if len(results) > 0 && results[0].ID != "fts1" {
		t.Errorf("Expected fts1 as top result, got %s", results[0].ID)
	}

	// Multi-word search
	results, err = SearchSQLite("ethereum upgrade", 10)
	if err != nil {
		t.Fatalf("FTS search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected FTS results for 'ethereum upgrade'")
	}
}
