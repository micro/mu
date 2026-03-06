package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
		`)
		if err != nil {
			initErr = fmt.Errorf("failed to create tables: %w", err)
			return
		}

		// Create FTS5 virtual table for full-text search
		_, err = db.Exec(`
			CREATE VIRTUAL TABLE IF NOT EXISTS index_fts USING fts5(
				title,
				content,
				content='index_entries',
				content_rowid='rowid'
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

	// Delete old FTS entry if exists (content= tables need manual sync)
	db.Exec(`DELETE FROM index_fts WHERE rowid = (SELECT rowid FROM index_entries WHERE id = ?)`, id)

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
		// Insert into FTS index
		db.Exec(`INSERT INTO index_fts(rowid, title, content) SELECT rowid, title, content FROM index_entries WHERE id = ?`, id)
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

// SearchSQLite performs full-text search using keyword matching.
// SQLite indexing never generates embeddings, so vector search is not attempted.
func SearchSQLite(query string, limit int, opts ...SearchOption) ([]*IndexEntry, error) {
	if _, err := getDB(); err != nil {
		return nil, err
	}

	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return searchSQLiteFallback(query, limit, options)
}

// searchSQLiteFallback uses FTS5 with LIKE fallback
func searchSQLiteFallback(query string, limit int, options *SearchOptions) ([]*IndexEntry, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var allEntries []*IndexEntry

	// Phase 1: FTS5 search (fast, ranked by relevance)
	ftsQuery := buildFTS5Query(words)
	if ftsQuery != "" {
		ftsSQL := `
			SELECT e.id, e.type, e.title, e.content, e.metadata, e.indexed_at
			FROM index_fts f
			JOIN index_entries e ON e.rowid = f.rowid
			WHERE index_fts MATCH ?`
		var ftsArgs []interface{}
		ftsArgs = append(ftsArgs, ftsQuery)
		if options.Type != "" {
			ftsSQL += ` AND e.type = ?`
			ftsArgs = append(ftsArgs, options.Type)
		}
		ftsSQL += ` ORDER BY rank LIMIT 50`

		rows, err := db.Query(ftsSQL, ftsArgs...)
		if err == nil {
			for rows.Next() {
				var e IndexEntry
				var meta sql.NullString
				var idx time.Time
				if rows.Scan(&e.ID, &e.Type, &e.Title, &e.Content, &meta, &idx) == nil {
					e.IndexedAt = idx
					if meta.Valid {
						json.Unmarshal([]byte(meta.String), &e.Metadata)
					}
					if !seen[e.ID] {
						seen[e.ID] = true
						allEntries = append(allEntries, &e)
					}
				}
			}
			rows.Close()
		}
	}

	// Phase 2: LIKE fallback for partial matches FTS5 might miss
	if len(allEntries) < limit {
		var likeConds []string
		var likeArgs []interface{}
		for _, word := range words {
			if len(word) < 2 {
				continue
			}
			likeConds = append(likeConds, "LOWER(title) LIKE ?")
			likeArgs = append(likeArgs, "%"+word+"%")
		}
		if len(likeConds) > 0 {
			where := strings.Join(likeConds, " OR ")
			if options.Type != "" {
				where = "(" + where + ") AND type = ?"
				likeArgs = append(likeArgs, options.Type)
			}
			likeArgs = append(likeArgs, 50)
			rows, err := db.Query(fmt.Sprintf(`
				SELECT id, type, title, content, metadata, indexed_at
				FROM index_entries WHERE %s
				ORDER BY indexed_at DESC LIMIT ?`, where), likeArgs...)
			if err == nil {
				for rows.Next() {
					var e IndexEntry
					var meta sql.NullString
					var idx time.Time
					if rows.Scan(&e.ID, &e.Type, &e.Title, &e.Content, &meta, &idx) == nil {
						e.IndexedAt = idx
						if meta.Valid {
							json.Unmarshal([]byte(meta.String), &e.Metadata)
						}
						if !seen[e.ID] {
							seen[e.ID] = true
							allEntries = append(allEntries, &e)
						}
					}
				}
				rows.Close()
			}
		}
	}

	// Score and sort all collected entries
	type scoredEntry struct {
		entry *IndexEntry
		score float64
	}
	var scored []scoredEntry
	for _, entry := range allEntries {
		score := scoreMatch(entry, words)
		if score > 0 {
			scored = append(scored, scoredEntry{entry, score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return getPostedAt(scored[i].entry).After(getPostedAt(scored[j].entry))
	})

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]*IndexEntry, len(scored))
	for i, s := range scored {
		results[i] = s.entry
	}

	return results, nil
}

// buildFTS5Query converts search words into an FTS5 query string
func buildFTS5Query(words []string) string {
	var terms []string
	for _, w := range words {
		if len(w) < 2 {
			continue
		}
		// Escape double quotes in terms
		w = strings.ReplaceAll(w, "\"", "")
		if w != "" {
			terms = append(terms, "\""+w+"\"")
		}
	}
	if len(terms) == 0 {
		return ""
	}
	// Use OR so any term matches
	return strings.Join(terms, " OR ")
}

// scoreMatch calculates relevance score for an entry
func scoreMatch(entry *IndexEntry, words []string) float64 {
	score := 0.0
	titleLower := strings.ToLower(entry.Title)
	contentLower := strings.ToLower(entry.Content)

	for _, word := range words {
		// Exact word boundary match in title (highest value)
		if matchesWordBoundary(titleLower, word) {
			score += 10.0
			fmt.Printf("[SCORE] Title word-boundary match '%s' in '%s' -> +10\n", word, entry.Title[:min(50, len(entry.Title))])
		} else if strings.Contains(titleLower, word) {
			// Substring match in title
			score += 3.0
		}

		// Word boundary match in content
		if matchesWordBoundary(contentLower, word) {
			score += 2.0
		} else if strings.Contains(contentLower, word) {
			// Substring match in content
			score += 0.5
		}
	}

	return score
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// matchesWordBoundary checks if word appears as a whole word (not substring)
func matchesWordBoundary(text, word string) bool {
	idx := 0
	for {
		pos := strings.Index(text[idx:], word)
		if pos == -1 {
			return false
		}
		pos += idx

		// Check character before
		validStart := pos == 0 || !isWordChar(text[pos-1])
		// Check character after
		endPos := pos + len(word)
		validEnd := endPos >= len(text) || !isWordChar(text[endPos])

		if validStart && validEnd {
			return true
		}

		idx = pos + 1
		if idx >= len(text) {
			return false
		}
	}
}

// isWordChar returns true for letters and numbers
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// getPostedAt extracts posted_at from metadata, falling back to IndexedAt
func getPostedAt(entry *IndexEntry) time.Time {
	if entry.Metadata == nil {
		return entry.IndexedAt
	}

	postedAt := entry.Metadata["posted_at"]
	if postedAt == nil {
		return entry.IndexedAt
	}

	switch v := postedAt.(type) {
	case time.Time:
		return v
	case string:
		// Try multiple formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t
			}
		}
	case float64:
		// Unix timestamp
		return time.Unix(int64(v), 0)
	}

	return entry.IndexedAt
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



// MigrateFromJSON migrates existing JSON data to SQLite
func MigrateFromJSON() error {
	db, err := getDB()
	if err != nil {
		return err
	}

	// Check if migration already done
	var indexCount int
	db.QueryRow(`SELECT COUNT(*) FROM index_entries`).Scan(&indexCount)
	if indexCount > 0 {
		fmt.Printf("[data] SQLite already has %d entries, skipping migration\n", indexCount)
		return nil
	}

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
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	fmt.Printf("[data] Migrated %d index entries\n", migrated)

	rebuildFTS()
	fmt.Println("[data] Migration complete!")

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

	return entries, 0, nil
}

// rebuildFTS repopulates the FTS5 index from the index_entries table
func rebuildFTS() {
	db, err := getDB()
	if err != nil {
		return
	}
	fmt.Println("[data] Rebuilding FTS index...")
	db.Exec(`DELETE FROM index_fts`)
	result, err := db.Exec(`INSERT INTO index_fts(rowid, title, content) SELECT rowid, title, content FROM index_entries`)
	if err != nil {
		fmt.Printf("[data] FTS rebuild error: %v\n", err)
		return
	}
	count, _ := result.RowsAffected()
	fmt.Printf("[data] FTS index rebuilt with %d entries\n", count)
}

// EnsureFTS checks if FTS index needs rebuilding on startup
func EnsureFTS() {
	db, err := getDB()
	if err != nil {
		return
	}
	var ftsCount, entryCount int
	db.QueryRow(`SELECT COUNT(*) FROM index_fts`).Scan(&ftsCount)
	db.QueryRow(`SELECT COUNT(*) FROM index_entries`).Scan(&entryCount)
	if entryCount > 0 && ftsCount == 0 {
		rebuildFTS()
	}
}
