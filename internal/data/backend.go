package data

// Which store the search index is actually in.
//
// There are two implementations and they leave two files: index.json, loaded
// into a map and rewritten whole, and index.db, an SQLite database with FTS5
// over it. Which one is live is a single environment variable read once at
// startup, and nothing said so anywhere — so an instance with a 32MB index.json
// and a 366MB index.db on disk gave no way to tell whether it was rewriting the
// first on every article the crawler found, or whether that file had been dead
// since the migration.
//
// It matters because the two failure modes look identical from outside and have
// opposite fixes: a hot JSON index is a write to remove, and a stale one is a
// file to delete.

// SearchBackend names the index that is being written, in words.
func SearchBackend() string {
	if UseSQLite {
		return "SQLite (index.db, FTS5)"
	}
	return "JSON (index.json, rewritten whole on every change)"
}

// Stale names the files on disk that the live backend does not write.
//
// Not deleted here — deciding that a file is rubbish is not the same as being
// the thing that removes it, and the one that is stale today is the archive if
// the setting is changed back.
func Stale() []string {
	if UseSQLite {
		return []string{"index.json", "embeddings.json"}
	}
	return []string{"index.db", "embeddings.json"}
}
