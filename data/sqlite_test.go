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

	// Create test embeddings.json
	testEmbeddings := `{
		"test1": [0.1, 0.2, 0.3, 0.4, 0.5],
		"test2": [0.5, 0.4, 0.3, 0.2, 0.1]
	}`
	os.WriteFile(filepath.Join(dataDir, "embeddings.json"), []byte(testEmbeddings), 0644)

	// Run migration
	err := MigrateFromJSON()
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify stats
	entries, embCount, err := GetIndexStats()
	if err != nil {
		t.Fatalf("GetIndexStats failed: %v", err)
	}
	if entries != 2 {
		t.Errorf("Expected 2 entries, got %d", entries)
	}
	if embCount != 2 {
		t.Errorf("Expected 2 embeddings, got %d", embCount)
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

	// Test embedding retrieval
	emb, err := GetEmbeddingSQLite("test1")
	if err != nil {
		t.Fatalf("GetEmbeddingSQLite failed: %v", err)
	}
	if len(emb) != 5 {
		t.Errorf("Expected 5 embedding values, got %d", len(emb))
	}
	if emb[0] != 0.1 {
		t.Errorf("Expected emb[0]=0.1, got %f", emb[0])
	}
}

func TestSQLiteSearch(t *testing.T) {
	// Use a temp directory for test
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Reset singleton
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
	// Use a temp directory for test
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Reset singleton
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
	if entry.Metadata["version"] != float64(2) { // JSON numbers become float64
		t.Errorf("Expected version 2, got %v", entry.Metadata["version"])
	}

	// Verify only one entry exists
	entries, _, _ := GetIndexStats()
	if entries != 1 {
		t.Errorf("Expected 1 entry after update, got %d", entries)
	}
}

func TestEmbeddingByteConversion(t *testing.T) {
	original := []float64{0.1, 0.2, 0.3, -0.4, 1.5, -2.6}

	bytes := float64SliceToBytes(original)
	restored := bytesToFloat64Slice(bytes)

	if len(restored) != len(original) {
		t.Fatalf("Length mismatch: %d vs %d", len(restored), len(original))
	}

	for i, v := range original {
		if restored[i] != v {
			t.Errorf("Value mismatch at %d: %f vs %f", i, restored[i], v)
		}
	}
}
