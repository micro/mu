package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLite database handle
var (
	db     *sql.DB
	dbOnce sync.Once
	dbPath string
)

// initDB initializes the SQLite database
func initDB() error {
	var initErr error
	dbOnce.Do(func() {
		dir := os.ExpandEnv("$HOME/.mu")
		dbPath = filepath.Join(dir, "data", "index.db")
		os.MkdirAll(filepath.Dir(dbPath), 0700)

		var err error
		db, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=10000")
		if err != nil {
			initErr = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// SQLite works best with limited connections
		db.SetMaxOpenConns(1) // Serialize all access to avoid locks
		db.SetMaxIdleConns(1)

		// Create tables
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS index_entries (
				id TEXT PRIMARY KEY,
				type TEXT NOT NULL,
				title TEXT NOT NULL,
				content TEXT NOT NULL,
				metadata TEXT,
				indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_type ON index_entries(type);
			CREATE INDEX IF NOT EXISTS idx_indexed_at ON index_entries(indexed_at);

			CREATE TABLE IF NOT EXISTS embeddings (
				id TEXT PRIMARY KEY,
				embedding BLOB NOT NULL,
				FOREIGN KEY (id) REFERENCES index_entries(id) ON DELETE CASCADE
			);
		`)
		if err != nil {
			initErr = fmt.Errorf("failed to create tables: %w", err)
			return
		}

		fmt.Println("[data] SQLite database initialized at", dbPath)
	})
	return initErr
}

// getDB returns the database handle, initializing if needed
func getDB() (*sql.DB, error) {
	if err := initDB(); err != nil {
		return nil, err
	}
	return db, nil
}

// IndexSQLite adds or updates an entry in the SQLite index
func IndexSQLite(id, entryType, title, content string, metadata map[string]interface{}) error {
	db, err := getDB()
	if err != nil {
		return err
	}

	var metadataJSON []byte
	if metadata != nil {
		metadataJSON, _ = json.Marshal(metadata)
	}

	_, err = db.Exec(`
		INSERT INTO index_entries (id, type, title, content, metadata, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			title = excluded.title,
			content = excluded.content,
			metadata = excluded.metadata,
			indexed_at = excluded.indexed_at
	`, id, entryType, title, content, string(metadataJSON), time.Now())

	if err == nil {
		// Publish event
		Publish(Event{
			Type: EventIndexComplete,
			Data: map[string]interface{}{
				"id":   id,
				"type": entryType,
			},
		})
	}

	return err
}

// GetByIDSQLite retrieves an entry by ID from SQLite
func GetByIDSQLite(id string) (*IndexEntry, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	var entry IndexEntry
	var metadataJSON sql.NullString
	var indexedAt time.Time

	err = db.QueryRow(`
		SELECT id, type, title, content, metadata, indexed_at
		FROM index_entries WHERE id = ?
	`, id).Scan(&entry.ID, &entry.Type, &entry.Title, &entry.Content, &metadataJSON, &indexedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	entry.IndexedAt = indexedAt
	if metadataJSON.Valid && metadataJSON.String != "" {
		json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
	}

	return &entry, nil
}

// SearchSQLite performs text search using LIKE (FTS5 not always available)
func SearchSQLite(query string, limit int, opts ...SearchOption) ([]*IndexEntry, error) {
	if _, err := getDB(); err != nil {
		return nil, err
	}

	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Use LIKE search (works everywhere)
	return searchSQLiteFallback(query, limit, options)
}

// searchSQLiteFallback uses LIKE when FTS fails
func searchSQLiteFallback(query string, limit int, options *SearchOptions) ([]*IndexEntry, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	// Split query into words and build WHERE clause that matches all words
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	// Build WHERE conditions: each word must appear in title OR content
	var conditions []string
	var args []interface{}
	for _, word := range words {
		if len(word) < 3 {
			continue // Skip very short words
		}
		likeWord := "%" + word + "%"
		conditions = append(conditions, "(title LIKE ? OR content LIKE ?)")
		args = append(args, likeWord, likeWord)
	}

	if len(conditions) == 0 {
		return nil, nil
	}

	whereClause := strings.Join(conditions, " AND ")
	if options.Type != "" {
		whereClause = "(" + whereClause + ") AND type = ?"
		args = append(args, options.Type)
	}
	args = append(args, limit)

	queryStr := fmt.Sprintf(`
		SELECT id, type, title, content, metadata, indexed_at
		FROM index_entries
		WHERE %s
		ORDER BY indexed_at DESC
		LIMIT ?
	`, whereClause)

	rows, queryErr := db.Query(queryStr, args...)
	if queryErr != nil {
		return nil, queryErr
	}
	defer rows.Close()

	var results []*IndexEntry
	for rows.Next() {
		var entry IndexEntry
		var metadataJSON sql.NullString
		var indexedAt time.Time

		err := rows.Scan(&entry.ID, &entry.Type, &entry.Title, &entry.Content, &metadataJSON, &indexedAt)
		if err != nil {
			continue
		}

		entry.IndexedAt = indexedAt
		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		results = append(results, &entry)
	}

	return results, nil
}

// GetByTypeSQLite returns entries of a specific type
func GetByTypeSQLite(entryType string, limit int) ([]*IndexEntry, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT id, type, title, content, metadata, indexed_at
		FROM index_entries
		WHERE type = ?
		ORDER BY indexed_at DESC
		LIMIT ?
	`, entryType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*IndexEntry
	for rows.Next() {
		var entry IndexEntry
		var metadataJSON sql.NullString
		var indexedAt time.Time

		err := rows.Scan(&entry.ID, &entry.Type, &entry.Title, &entry.Content, &metadataJSON, &indexedAt)
		if err != nil {
			continue
		}

		entry.IndexedAt = indexedAt
		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		results = append(results, &entry)
	}

	return results, nil
}

// SaveEmbeddingSQLite stores an embedding in SQLite
func SaveEmbeddingSQLite(id string, embedding []float64) error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Convert float64 slice to bytes (more compact than JSON)
	embBytes := float64SliceToBytes(embedding)

	_, err = db.Exec(`
		INSERT INTO embeddings (id, embedding)
		VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET embedding = excluded.embedding
	`, id, embBytes)

	return err
}

// GetEmbeddingSQLite retrieves an embedding from SQLite
func GetEmbeddingSQLite(id string) ([]float64, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	var embBytes []byte
	err = db.QueryRow(`SELECT embedding FROM embeddings WHERE id = ?`, id).Scan(&embBytes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return bytesToFloat64Slice(embBytes), nil
}

// VectorSearchSQLite performs vector similarity search
func VectorSearchSQLite(queryEmbedding []float64, limit int, opts ...SearchOption) ([]*IndexEntry, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Get all embeddings and compute similarity
	// (For a proper solution, use sqlite-vec extension)
	var rows *sql.Rows
	if options.Type != "" {
		rows, err = db.Query(`
			SELECT e.id, e.type, e.title, e.content, e.metadata, e.indexed_at, emb.embedding
			FROM index_entries e
			JOIN embeddings emb ON e.id = emb.id
			WHERE e.type = ?
		`, options.Type)
	} else {
		rows, err = db.Query(`
			SELECT e.id, e.type, e.title, e.content, e.metadata, e.indexed_at, emb.embedding
			FROM index_entries e
			JOIN embeddings emb ON e.id = emb.id
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scoredEntry struct {
		entry *IndexEntry
		score float64
	}
	var scored []scoredEntry

	for rows.Next() {
		var entry IndexEntry
		var metadataJSON sql.NullString
		var indexedAt time.Time
		var embBytes []byte

		err := rows.Scan(&entry.ID, &entry.Type, &entry.Title, &entry.Content, &metadataJSON, &indexedAt, &embBytes)
		if err != nil {
			continue
		}

		entry.IndexedAt = indexedAt
		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		emb := bytesToFloat64Slice(embBytes)
		if len(emb) > 0 {
			sim := cosineSimilarity(queryEmbedding, emb)
			if sim > 0.3 {
				scored = append(scored, scoredEntry{entry: &entry, score: sim})
			}
		}
	}

	// Sort by similarity
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Return top N
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]*IndexEntry, len(scored))
	for i, s := range scored {
		results[i] = s.entry
	}

	return results, nil
}

// MigrateFromJSON migrates existing JSON data to SQLite
func MigrateFromJSON() error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Check if migration already done
	var indexCount, embCount int
	db.QueryRow(`SELECT COUNT(*) FROM index_entries`).Scan(&indexCount)
	db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&embCount)

	if indexCount > 0 && embCount > 0 {
		fmt.Printf("[data] SQLite already has %d entries and %d embeddings, skipping migration\n", indexCount, embCount)
		return nil
	}

	if indexCount > 0 {
		fmt.Printf("[data] SQLite has %d entries but %d embeddings, migrating embeddings only\n", indexCount, embCount)
		return migrateEmbeddings()
	}

	fmt.Println("[data] Starting migration from JSON to SQLite...")

	// Load existing index.json
	b, err := LoadFile("index.json")
	if err != nil {
		fmt.Println("[data] No index.json to migrate")
		return nil
	}

	var oldIndex map[string]*struct {
		ID        string                 `json:"id"`
		Type      string                 `json:"type"`
		Title     string                 `json:"title"`
		Content   string                 `json:"content"`
		Metadata  map[string]interface{} `json:"metadata,omitempty"`
		IndexedAt time.Time              `json:"indexed_at"`
	}

	if err := json.Unmarshal(b, &oldIndex); err != nil {
		return fmt.Errorf("failed to parse index.json: %w", err)
	}

	fmt.Printf("[data] Migrating %d index entries...\n", len(oldIndex))

	// Begin transaction for faster inserts
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO index_entries (id, type, title, content, metadata, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	migrated := 0
	for id, entry := range oldIndex {
		var metadataJSON []byte
		if entry.Metadata != nil {
			metadataJSON, _ = json.Marshal(entry.Metadata)
		}

		_, err := stmt.Exec(id, entry.Type, entry.Title, entry.Content, string(metadataJSON), entry.IndexedAt)
		if err != nil {
			fmt.Printf("[data] Failed to migrate entry %s: %v\n", id, err)
			continue
		}
		migrated++

		if migrated%1000 == 0 {
			fmt.Printf("[data] Migrated %d entries...\n", migrated)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit index migration: %w", err)
	}

	fmt.Printf("[data] Migrated %d index entries\n", migrated)

	// Migrate embeddings
	b, err = LoadFile("embeddings.json")
	if err != nil {
		fmt.Println("[data] No embeddings.json to migrate")
		return nil
	}

	var oldEmbeddings map[string][]float64
	if err := json.Unmarshal(b, &oldEmbeddings); err != nil {
		return fmt.Errorf("failed to parse embeddings.json: %w", err)
	}

	fmt.Printf("[data] Migrating %d embeddings...\n", len(oldEmbeddings))

	tx, err = db.Begin()
	if err != nil {
		return err
	}

	stmt, err = tx.Prepare(`INSERT OR REPLACE INTO embeddings (id, embedding) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	embMigrated := 0
	for id, emb := range oldEmbeddings {
		embBytes := float64SliceToBytes(emb)
		_, err := stmt.Exec(id, embBytes)
		if err != nil {
			fmt.Printf("[data] Failed to migrate embedding %s: %v\n", id, err)
			continue
		}
		embMigrated++

		if embMigrated%1000 == 0 {
			fmt.Printf("[data] Migrated %d embeddings...\n", embMigrated)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit embeddings migration: %w", err)
	}

	fmt.Printf("[data] Migrated %d embeddings\n", embMigrated)
	fmt.Println("[data] Migration complete!")

	return nil
}

// migrateEmbeddings migrates only embeddings (when index already exists)
func migrateEmbeddings() error {
	db, err := getDB()
	if err != nil {
		return err
	}

	b, err := LoadFile("embeddings.json")
	if err != nil {
		fmt.Println("[data] No embeddings.json to migrate")
		return nil
	}

	var oldEmbeddings map[string][]float64
	if err := json.Unmarshal(b, &oldEmbeddings); err != nil {
		return fmt.Errorf("failed to parse embeddings.json: %w", err)
	}

	fmt.Printf("[data] Migrating %d embeddings...\n", len(oldEmbeddings))

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO embeddings (id, embedding) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	embMigrated := 0
	for id, emb := range oldEmbeddings {
		embBytes := float64SliceToBytes(emb)
		_, err := stmt.Exec(id, embBytes)
		if err != nil {
			fmt.Printf("[data] Failed to migrate embedding %s: %v\n", id, err)
			continue
		}
		embMigrated++

		if embMigrated%1000 == 0 {
			fmt.Printf("[data] Migrated %d embeddings...\n", embMigrated)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit embeddings migration: %w", err)
	}

	fmt.Printf("[data] Migrated %d embeddings\n", embMigrated)
	return nil
}

// GetIndexStats returns statistics about the SQLite index
func GetIndexStats() (entries int, embeddingCount int, err error) {
	db, err := getDB()
	if err != nil {
		return 0, 0, err
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM index_entries`).Scan(&entries)
	if err != nil {
		return 0, 0, err
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&embeddingCount)
	if err != nil {
		return entries, 0, err
	}

	return entries, embeddingCount, nil
}

// Helper functions for embedding byte conversion
func float64SliceToBytes(f []float64) []byte {
	b := make([]byte, len(f)*8)
	for i, v := range f {
		bits := math.Float64bits(v)
		b[i*8] = byte(bits)
		b[i*8+1] = byte(bits >> 8)
		b[i*8+2] = byte(bits >> 16)
		b[i*8+3] = byte(bits >> 24)
		b[i*8+4] = byte(bits >> 32)
		b[i*8+5] = byte(bits >> 40)
		b[i*8+6] = byte(bits >> 48)
		b[i*8+7] = byte(bits >> 56)
	}
	return b
}

func bytesToFloat64Slice(b []byte) []float64 {
	if len(b)%8 != 0 {
		return nil
	}
	f := make([]float64, len(b)/8)
	for i := range f {
		bits := uint64(b[i*8]) |
			uint64(b[i*8+1])<<8 |
			uint64(b[i*8+2])<<16 |
			uint64(b[i*8+3])<<24 |
			uint64(b[i*8+4])<<32 |
			uint64(b[i*8+5])<<40 |
			uint64(b[i*8+6])<<48 |
			uint64(b[i*8+7])<<56
		f[i] = math.Float64frombits(bits)
	}
	return f
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilaritySQLite(a, b []float64) float64 {
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
