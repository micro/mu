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
	Owner       string // Account scope for private entries (see WithOwner)
	KeywordOnly bool   // Use keyword matching only
}

// SearchOption is a functional option for configuring search
type SearchOption func(*SearchOptions)

// WithType filters search results by entry type
func WithType(entryType string) SearchOption {
	return func(opts *SearchOptions) {
		opts.Type = entryType
	}
}

// WithOwner scopes a search to the given account for private entries.
//
// Index entries with an empty Owner are public and always returned. Entries
// with a non-empty Owner are private and are ONLY returned when the caller
// passes WithOwner(thatAccount). The safe default (no WithOwner) therefore
// excludes ALL private entries, so existing callers can never surface another
// account's private content (e.g. mail) by accident.
func WithOwner(accountID string) SearchOption {
	return func(opts *SearchOptions) {
		opts.Owner = accountID
	}
}

// WithKeywordOnly uses keyword matching only
func WithKeywordOnly() SearchOption {
	return func(opts *SearchOptions) {
		opts.KeywordOnly = true
	}
}

// dataPath resolves key under the data dir and confines it there, rejecting any
// key that would escape the store (via "..", an absolute path, etc.). This is a
// defense-in-depth guard: callers that build keys from user-influenced input
// (app slugs, collection names) cannot cause reads or writes outside the store,
// even if a caller forgets to validate its inputs.
func dataPath(key string) (string, error) {
	base := filepath.Join(os.ExpandEnv("$HOME/.mu"), "data")
	file := filepath.Join(base, key)
	rel, err := filepath.Rel(base, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("data: invalid key %q", key)
	}
	return file, nil
}

// writeAtomic writes b to file durably: it writes a temp file in the same
// directory, fsyncs it, then renames it into place. Rename within a directory
// is atomic on POSIX, so a crash, a full disk, or a concurrent reader can never
// observe a half-written file — the previous contents survive intact instead.
//
// This matters because the store keeps whole-file blobs: without this, an
// interrupted write to accounts.json or wallets.json truncates every account or
// balance in it. Mode is 0600 throughout: these files hold credentials,
// sessions, tokens, passkeys and wallet state.
func writeAtomic(file string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we fail before the rename succeeds.
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	// fsync before rename so the data is on disk, not just in the page cache.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, file); err != nil {
		return err
	}
	tmpName = "" // renamed; nothing to clean up
	return nil
}

// SaveFile saves data to disk
func SaveFile(key, val string) error {
	file, err := dataPath(key)
	if err != nil {
		return err
	}
	return writeAtomic(file, []byte(val))
}

// LoadFile loads a file from disk
func LoadFile(key string) ([]byte, error) {
	file, err := dataPath(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(file)
}

// ListKeys returns the file names directly under a key prefix, without their
// directories. Missing means empty rather than an error: a store nobody has
// written to yet has no directory, and that is not a failure.
//
// Confined by dataPath like every other key, so a prefix cannot escape the data
// directory.
func ListKeys(prefix string) ([]string, error) {
	dir, err := dataPath(prefix)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func DeleteFile(key string) error {
	file, err := dataPath(key)
	if err != nil {
		return err
	}
	return os.Remove(file)
}

func SaveJSON(key string, val interface{}) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	file, err := dataPath(key)
	if err != nil {
		return err
	}
	return writeAtomic(file, b)
}

func LoadJSON(key string, val interface{}) error {
	file, err := dataPath(key)
	if err != nil {
		return err
	}
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
	Owner    string
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
	Owner     string                 `json:"owner,omitempty"` // account scope; empty = public
	Metadata  map[string]interface{} `json:"metadata"`
	IndexedAt time.Time              `json:"indexed_at"`
}

// SearchResult represents a search hit with relevance score
type SearchResult struct {
	Entry *IndexEntry
	Score float64
}

// Index queues a public entry to be added or updated in the search index.
func Index(id, entryType, title, content string, metadata map[string]interface{}) {
	IndexOwned(id, entryType, title, content, "", metadata)
}

// IndexOwned indexes an entry scoped to a specific account. A non-empty owner
// marks the entry private: it is only returned by searches that pass
// WithOwner(owner). Pass an empty owner for public content.
func IndexOwned(id, entryType, title, content, owner string, metadata map[string]interface{}) {
	// Use SQLite backend if enabled
	if UseSQLite {
		if err := IndexSQLite(id, entryType, title, content, owner, metadata); err != nil {
			fmt.Printf("[data] SQLite index error: %v\n", err)
		}
		return
	}

	work := IndexWork{
		ID:       id,
		Type:     entryType,
		Title:    title,
		Content:  content,
		Owner:    owner,
		Metadata: metadata,
	}

	// Queue the work instead of processing immediately
	select {
	case indexWorkQueue <- work:
		// Work queued successfully
	default:
		// Queue full, process synchronously to avoid dropping
		processIndexWork(work)
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
		Owner:     work.Owner,
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

		// Owner scoping: never surface another account's private entries.
		// Public entries (empty Owner) are always eligible.
		if entry.Owner != "" && entry.Owner != options.Owner {
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
		// GetByType is public-only: private entries are excluded so callers
		// (e.g. topic generation) can never pull another account's content.
		if entry.Type == entryType && entry.Owner == "" {
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

// saveDebounce is how long a save waits to batch further updates. A variable so
// a test need not spend a real second proving where a write lands.
//
// Read and written under saveMutex, which costs nothing here and is not
// optional: a save is a goroutine that outlives the call that started it, so a
// test setting this while a previous test's save is still sleeping on it is a
// real race and the detector says so. Under -race the whole package failed on
// it, in a test that passed on its own.
var saveDebounce = time.Second

// saveQueued is called once a save has decided where it is going and before it
// waits. Nil in normal operation, and a nil check is its whole cost. Same lock,
// for the same reason.
//
// It exists because the property worth testing here — that the destination is
// fixed at queue time rather than at write time — is only observable in the gap
// between the two, and a test that tried to hit that gap by sleeping would be
// testing the scheduler.
var saveQueued func()

// saveIndex persists the index to disk, batching updates that arrive together.
//
// The destination is resolved before the wait, not after, and that is the whole
// point of the two lines rather than one. dataPath reads $HOME every time it is
// called, so resolving it after a one-second sleep meant a pending write
// followed $HOME wherever it had got to — it wrote the index into whichever
// data directory happened to be current when the goroutine woke up, not the one
// that was current when the write was queued.
//
// In production $HOME does not move and this never showed. In the test binary
// every test sets its own, and the result was a real CI failure that read as
// something else entirely: TestSQLiteMigration wrote a two-entry index.json into
// its own temp directory, an earlier test's debounced save landed on top of it,
// and the test reported "Expected 2 entries, got 3" and "Entry not found" — a
// migration bug on the face of it, and actually a write to the wrong directory.
// The fix is to decide where a write is going when it is queued.
func saveIndex() {
	// Debounce saves - only save once even if called multiple times
	saveMutex.Lock()
	if savePending {
		saveMutex.Unlock()
		return
	}
	savePending = true
	// Taken now, with the lock already held, so this goroutine sleeps on the
	// value that was current when it was queued rather than one a later caller
	// installs while it waits.
	debounce, queued := saveDebounce, saveQueued
	saveMutex.Unlock()

	// Where this is going is decided now, while the caller's data directory is
	// still the current one.
	file, err := dataPath("index.json")
	if queued != nil {
		queued()
	}

	// Wait a bit to batch multiple index updates
	time.Sleep(debounce)

	if err == nil {
		// Marshalled under the read lock, written outside it: the disk write
		// does not need to hold up every reader of the index.
		indexMutex.RLock()
		b, mErr := json.Marshal(index)
		indexMutex.RUnlock()
		if mErr == nil {
			writeAtomic(file, b) //nolint:errcheck
		}
	}

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
