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

import "os"

// SearchBackend names the index that is being written, in words.
func SearchBackend() string {
	if UseSQLite {
		return "SQLite (index.db, FTS5)"
	}
	return "JSON (index.json, rewritten whole on every change)"
}

// Stale names the files on disk that the live backend does not write. For
// display: a large file nothing writes is an archive, not a cost, and the two
// are indistinguishable from a size alone.
func Stale() []string {
	if UseSQLite {
		return []string{"index.json", "embeddings.json"}
	}
	return []string{"index.db", "embeddings.json"}
}

// Superseded names the files that are both unwritten *and* safe to lose,
// because everything in them is in the live store.
//
// Deliberately not the same list as Stale, and the difference is the whole
// point of having two. With SQLite off, index.db is unwritten and is also six
// months of archive that nothing else holds — offering to delete it because it
// is "stale" would be the worst button in the product. Superseded is only ever
// the other direction: the migration ran, index_entries is not empty, so the
// JSON the migration read is a copy of what SQLite now has.
//
// Empty when there is any doubt at all, which is the only safe default for a
// function whose caller deletes what it returns.
func Superseded() []string {
	if !UseSQLite {
		return nil
	}
	// The migration is guarded on this count and skips when it is non-zero, so
	// a populated table is the proof that index.json has already been read and
	// is not being read again. Without it, index.json is still the input to a
	// migration that has not happened.
	entries, _, err := IndexStats()
	if err != nil || entries == 0 {
		return nil
	}
	// embeddings.json is written by nothing in this codebase at all — the code
	// that produced it was removed, and the file stayed.
	return []string{"index.json", "embeddings.json"}
}

// RemoveSuperseded deletes what Superseded names, and reports what went and how
// much it freed.
//
// Every store here is rewritten whole, so a file nothing writes costs nothing
// per day — but it is on the disk, in the backup, and in every restore, and the
// only way to be rid of it was to go to the box with rm. That is a bad reason
// to carry a hundred megabytes.
func RemoveSuperseded() (removed []string, freed int64, err error) {
	for _, name := range Superseded() {
		path, perr := dataPath(name)
		if perr != nil {
			continue
		}
		info, serr := os.Stat(path)
		if serr != nil {
			continue // already gone
		}
		if rerr := os.Remove(path); rerr != nil {
			err = rerr
			continue
		}
		removed = append(removed, name)
		freed += info.Size()
	}
	return removed, freed, err
}
