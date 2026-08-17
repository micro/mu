package backup

// A backup nobody has restored from is a hope. These are the closest thing to
// restoring: that a snapshot contains what was there, that a later one does not
// lose it, and that pruning keeps what it says it keeps.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	Home = func() string { return dir }
	t.Cleanup(func() { Home = func() string { return os.ExpandEnv("$HOME/.mu") } })
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	lastRun = time.Time{}
	return dir
}

func write(t *testing.T, home, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "data", name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestASnapshotHasWhatWasThere(t *testing.T) {
	home := sandbox(t)
	write(t, home, "threads.json", `{"threads":[{"id":"one"}]}`)
	write(t, home, "notes.json", `{"a":"remember this"}`)

	snap, err := Take()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files != 2 {
		t.Fatalf("snapshot has %d files, want 2", snap.Files)
	}
	b, err := os.ReadFile(filepath.Join(Dir(), snap.Name, "notes.json"))
	if err != nil {
		t.Fatalf("a file that was there is not in the snapshot: %v", err)
	}
	if string(b) != `{"a":"remember this"}` {
		t.Errorf("the snapshot holds %q", b)
	}
}

// The older snapshot keeps the old contents after the file changes.
//
// This is the property the whole scheme rests on: writes rename over the old
// file rather than editing it, so a hardlink made earlier still points at what
// was there. If that were not true, every snapshot would silently become a copy
// of the present.
func TestAnOlderSnapshotStillHasTheOldContents(t *testing.T) {
	home := sandbox(t)
	write(t, home, "wallets.json", `{"balance":100}`)

	first, err := Take()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // snapshot names have second resolution

	// A write, the way the data package does it: a new file renamed into place.
	tmp := filepath.Join(home, "data", ".tmp")
	if err := os.WriteFile(tmp, []byte(`{"balance":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(home, "data", "wallets.json")); err != nil {
		t.Fatal(err)
	}

	second, err := Take()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(Dir(), first.Name, "wallets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != `{"balance":100}` {
		t.Errorf("the earlier snapshot now reads %q — it followed the live file, so "+
			"there is no history and a bad write is not recoverable", before)
	}
	after, _ := os.ReadFile(filepath.Join(Dir(), second.Name, "wallets.json"))
	if string(after) != `{"balance":0}` {
		t.Errorf("the newer snapshot reads %q, want the current contents", after)
	}
}

// An unchanged file is linked rather than copied.
func TestAnUnchangedFileIsNotCopiedTwice(t *testing.T) {
	home := sandbox(t)
	write(t, home, "steady.json", `{"unchanged":true}`)

	first, err := Take()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // snapshot names have second resolution
	second, err := Take()
	if err != nil {
		t.Fatal(err)
	}

	a, err := os.Stat(filepath.Join(Dir(), first.Name, "steady.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(Dir(), second.Name, "steady.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Error("an unchanged file was copied rather than linked — a month of " +
			"snapshots then costs a month of copies, and nobody keeps them")
	}
}

// The copies the data package sets aside are not themselves backed up.
func TestSetAsideFilesAreNotSnapshotted(t *testing.T) {
	home := sandbox(t)
	write(t, home, "real.json", `{}`)
	write(t, home, "real.json.prev", `{}`)
	write(t, home, "real.json.corrupt.20260101-000000", `{`)

	snap, err := Take()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files != 1 {
		t.Errorf("snapshot has %d files, want 1 — backing up the backups doubles "+
			"the directory for nothing", snap.Files)
	}
}

func TestPruningKeepsTheRecentOnes(t *testing.T) {
	home := sandbox(t)
	write(t, home, "a.json", `{}`)

	// More snapshots than the recent limit, dated by hand.
	for i := 0; i < keepRecent+5; i++ {
		at := time.Now().UTC().Add(-time.Duration(i) * time.Hour)
		if err := os.MkdirAll(filepath.Join(Dir(), at.Format("20060102-150405")), 0700); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	prune()
	mu.Unlock()

	got := List()
	if len(got) < keepRecent {
		t.Errorf("pruning left %d snapshots, want at least the %d recent ones",
			len(got), keepRecent)
	}
	// And the newest is still there, which is the one that matters most.
	if len(got) > 0 && time.Since(got[0].At) > 2*time.Hour {
		t.Error("the newest snapshot was pruned")
	}
}
