// Package backup keeps copies of the data directory, so a bad write is an
// afternoon rather than a year.
//
// Three different problems wear the word "backup" and only one of them is
// solved here:
//
//	a bad write, or an operator mistake   → frequent local snapshots, with history
//	the disk or the instance is gone      → a copy somewhere else entirely
//	somebody hostile has root             → a copy somewhere else, that root cannot delete
//
// This is the first. It is the one that has actually happened — a wallet was
// destroyed by two components writing the same file — and it is the one that
// needs no configuration, no credentials and no second machine, so every
// self-hosted instance gets it. The other two are `mu backup` and somebody's
// crontab, because an instance cannot be given somebody else's storage.
//
// # Hardlinks, and why they are honest here
//
// A snapshot links to files that have not changed rather than copying them.
// That is safe because of how writes work: SaveJSON writes a temp file and
// renames it over the old one, which replaces the directory entry and leaves
// the old inode alone. A link made yesterday still points at yesterday's
// contents. So a snapshot of a large data directory in which one file changed
// costs one file, and the ones before it stay exactly as they were.
//
// It also means the snapshots stay ordinary files. An operator can grep them,
// diff them, and copy one back with cp — which is most of the value of keeping
// data as JSON in the first place, and would be lost if these were archives.
//
// # What it does not promise
//
// A snapshot is consistent per file, not across files. Each file is whole,
// because each write is atomic; but a snapshot taken mid-flight can catch one
// store updated and a related one not. Freezing every writer for the duration
// would fix it and is not worth it at this size.
package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Interval is how often a snapshot is taken.
//
// Daily. Hourly was the first guess and it was wrong on a small disk: the point
// of a *local* copy is undoing a bad write or a mistake, and the finest-grained
// protection against that is already elsewhere — SaveJSON keeps a .prev the
// moment a store loses half of itself, which is closer to the accident than any
// snapshot could be. What these add is yesterday, and last week.
//
// Depth belongs off the box, where disk is somebody else's and losing the
// machine does not take the backups with it.
const Interval = 24 * time.Hour

// What is kept: recent snapshots in full, older ones thinning to one a day, and
// under all of it a hard ceiling on bytes.
const (
	keepRecent = 7
	keepDays   = 30
)

// MaxBytes is the ceiling on the whole snapshot directory.
//
// A count is not a budget. Seven snapshots of a small instance is nothing and
// seven of a large one fills the disk, and the disk filling takes the instance
// with it — a backup that stops the thing it protects is worse than no backup.
// So the oldest are dropped until the total fits, whatever the count says.
var MaxBytes int64 = 256 << 20

// IndexSnapshot copies the search index consistently, when something can.
//
// Filled in by internal/data, which holds the database handle. A plain file
// copy of a live SQLite database can catch it mid-transaction; VACUUM INTO
// cannot. Nil means copy the file and accept the risk, which is what an
// instance with no index does anyway.
var IndexSnapshot func(dst string) error

var (
	mu      sync.Mutex
	lastRun time.Time
)

// Home is the directory being protected. A variable so a test can point it
// somewhere harmless.
var Home = func() string {
	return os.ExpandEnv("$HOME/.mu")
}

// Dir is where snapshots go.
func Dir() string { return filepath.Join(Home(), "backup") }

// Snapshot is one point in time.
type Snapshot struct {
	Name  string // the directory name, which is its timestamp
	At    time.Time
	Files int
	Bytes int64 // apparent size; hardlinked files cost nothing on disk
}

// Load starts taking snapshots.
func Load() {
	go func() {
		// One at startup, because the most valuable snapshot is the one taken
		// before whatever is about to go wrong.
		if _, err := Take(); err != nil {
			fmt.Fprintf(os.Stderr, "[backup] %v\n", err)
		}
		for range time.Tick(Interval) {
			if _, err := Take(); err != nil {
				fmt.Fprintf(os.Stderr, "[backup] %v\n", err)
			}
		}
	}()
}

// Take makes a snapshot now and prunes the old ones.
func Take() (Snapshot, error) {
	mu.Lock()
	defer mu.Unlock()

	src := filepath.Join(Home(), "data")
	if _, err := os.Stat(src); err != nil {
		return Snapshot{}, fmt.Errorf("nothing to back up: %w", err)
	}

	// The newest existing snapshot is what unchanged files link to — found
	// before the new directory exists, or the new one is itself the newest and
	// everything links against an empty directory, which silently means
	// everything is copied.
	var prev string
	if all := list(); len(all) > 0 {
		prev = filepath.Join(Dir(), all[0].Name)
	}

	at := time.Now().UTC()
	name := at.Format("20060102-150405")
	dst := filepath.Join(Dir(), name)
	if err := os.MkdirAll(dst, 0700); err != nil {
		return Snapshot{}, err
	}

	snap := Snapshot{Name: name, At: at}
	entries, err := os.ReadDir(src)
	if err != nil {
		return Snapshot{}, err
	}
	for _, e := range entries {
		if e.IsDir() || skip(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())

		if err := copyOrLink(from, to, prev, e.Name(), info); err != nil {
			fmt.Fprintf(os.Stderr, "[backup] %s: %v\n", e.Name(), err)
			continue
		}
		snap.Files++
		snap.Bytes += info.Size()
	}

	// The search index is not here, and that is deliberate.
	//
	// It is the largest thing in the directory by far, it changes constantly,
	// and VACUUM writes a fresh file every time — so it can never be hardlinked
	// and every snapshot costs another whole copy of it. That is what turned a
	// small data directory into hundreds of megabytes of backups.
	//
	// It also protects nothing. These snapshots exist for a bad write to a JSON
	// store; nothing writes the index that way. The event that would lose the
	// index is losing the disk, and losing the disk loses these snapshots too.
	// So the index belongs in the off-box copy, which is the only thing that
	// survives that event, and nowhere else. See IndexSnapshot, which is what
	// that copy uses.

	lastRun = at
	prune()
	fit()
	return snap, nil
}

// copyOrLink links to the previous snapshot's copy when the file has not
// changed, and copies otherwise.
func copyOrLink(from, to, prev, name string, info os.FileInfo) error {
	if prev != "" {
		old := filepath.Join(prev, name)
		if s, err := os.Stat(old); err == nil &&
			s.Size() == info.Size() && s.ModTime().Equal(info.ModTime()) {
			if err := os.Link(old, to); err == nil {
				return nil
			}
			// Linking can fail across filesystems or on some mounts. Copying
			// is always correct, only more expensive.
		}
	}
	if err := copyFile(from, to); err != nil {
		return err
	}
	// Keep the source's timestamp on the copy.
	//
	// Not decoration: the next snapshot decides whether a file changed by
	// comparing size and modification time against this one. A copy stamped
	// with the time it was copied never matches, so every snapshot copies
	// everything and the linking is silently dead. It also makes a snapshot
	// read correctly — the dates are when the data was written, not when it
	// was backed up.
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func indexInto(src, dst string) error {
	if _, err := os.Stat(filepath.Join(src, "index.db")); err != nil {
		return nil // no index on this instance
	}
	to := filepath.Join(dst, "index.db")
	if IndexSnapshot != nil {
		return IndexSnapshot(to)
	}
	return copyFile(filepath.Join(src, "index.db"), to)
}

// skip leaves out what is not worth keeping: the temporary files an atomic
// write leaves behind, and the copies the data package sets aside — those are
// already the thing being protected, and backing up a backup doubles the
// directory for nothing.
func skip(name string) bool {
	switch {
	case strings.HasPrefix(name, "."):
		return true
	case strings.HasSuffix(name, ".prev"), strings.Contains(name, ".corrupt."):
		return true
	case name == "index.db", strings.HasSuffix(name, "-wal"), strings.HasSuffix(name, "-shm"):
		// The search index and its sidecars. See Take: it is the biggest thing
		// here, it can never be hardlinked, and a local copy of it protects
		// nothing that losing the disk would not take too.
		return true
	}
	return false
}

// List returns the snapshots, newest first.
func List() []Snapshot {
	mu.Lock()
	defer mu.Unlock()
	return list()
}

func list() []Snapshot {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		return nil
	}
	var out []Snapshot
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		at, err := time.Parse("20060102-150405", e.Name())
		if err != nil {
			continue
		}
		s := Snapshot{Name: e.Name(), At: at.UTC()}
		if files, err := os.ReadDir(filepath.Join(Dir(), e.Name())); err == nil {
			for _, f := range files {
				if info, err := f.Info(); err == nil {
					s.Files++
					s.Bytes += info.Size()
				}
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Last is when a snapshot was last taken, for a page that has to say whether
// this is working.
func Last() time.Time {
	mu.Lock()
	defer mu.Unlock()
	if !lastRun.IsZero() {
		return lastRun
	}
	if all := list(); len(all) > 0 {
		return all[0].At
	}
	return time.Time{}
}

// fit drops the oldest snapshots until the directory is under MaxBytes.
//
// After prune, not instead of it: prune expresses what is worth keeping and
// this expresses what there is room for. The newest is never dropped — a
// backup directory with nothing in it is the one outcome worse than a large
// one. Caller holds mu.
func fit() {
	if MaxBytes <= 0 {
		return
	}
	all := list()
	var total int64
	for _, s := range all {
		total += s.Bytes
	}
	for i := len(all) - 1; i > 0 && total > MaxBytes; i-- {
		os.RemoveAll(filepath.Join(Dir(), all[i].Name))
		total -= all[i].Bytes
	}
}

// prune keeps the recent ones in full and thins the rest to one a day.
//
// Losing an hour of granularity from last month is not the same kind of loss as
// losing last month, so the old ones thin rather than disappear. Caller holds
// mu.
func prune() {
	all := list()
	if len(all) <= keepRecent {
		return
	}
	seen := map[string]bool{}
	for i, s := range all {
		if i < keepRecent {
			seen[s.At.Format("2006-01-02")] = true
			continue
		}
		day := s.At.Format("2006-01-02")
		tooOld := time.Since(s.At) > time.Duration(keepDays)*24*time.Hour
		if seen[day] || tooOld {
			os.RemoveAll(filepath.Join(Dir(), s.Name))
			continue
		}
		seen[day] = true
	}
}
