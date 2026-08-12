package data

// Where a debounced write lands.
//
// saveIndex waits before writing, so that a burst of index updates costs one
// disk write. It used to resolve the destination after that wait — and dataPath
// reads $HOME every time it is called, so a pending write followed $HOME
// wherever it had got to in the meantime.
//
// Nothing in production moves $HOME, so this was invisible there. In the test
// binary every test sets its own, and it produced a CI failure that looked like
// something else: TestSQLiteMigration wrote a two-entry index.json into its own
// temp directory, an earlier test's pending save overwrote it, and the failure
// read "Expected 2 entries, got 3" and "Entry not found". Two assertions about
// migration, caused by a write going to the wrong directory.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// quickDebounce makes the wait short enough to test and long enough to still be
// a wait, and resets the shared save state so one test cannot silence the next.
func quickDebounce(t *testing.T) {
	t.Helper()
	orig := saveDebounce
	saveDebounce = 50 * time.Millisecond
	saveMutex.Lock()
	savePending = false
	saveMutex.Unlock()
	t.Cleanup(func() {
		saveDebounce = orig
		saveMutex.Lock()
		savePending = false
		saveMutex.Unlock()
	})
}

// start begins a save and returns once it has decided where it is going, so a
// test can change $HOME underneath it without racing the scheduler.
func start(t *testing.T) chan struct{} {
	t.Helper()
	resolved := make(chan struct{})
	saveQueued = func() { close(resolved) }
	t.Cleanup(func() { saveQueued = nil })

	done := make(chan struct{})
	go func() { saveIndex(); close(done) }()
	<-resolved
	return done
}

func readIndex(t *testing.T, home string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, ".mu", "data", "index.json"))
	if err != nil {
		return nil
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("index.json in %s is not JSON: %v", home, err)
	}
	return out
}

// TestADebouncedSaveGoesWhereItWasQueued.
func TestADebouncedSaveGoesWhereItWasQueued(t *testing.T) {
	quickDebounce(t)

	queued, later := t.TempDir(), t.TempDir()
	t.Setenv("HOME", queued)

	indexMutex.Lock()
	index = map[string]*IndexEntry{"queued": {ID: "queued", Type: "news", Title: "Queued here"}}
	indexMutex.Unlock()

	done := start(t)

	// While the save is still waiting, somebody else takes over $HOME — which
	// in the test binary is simply the next test starting.
	t.Setenv("HOME", later)
	<-done

	if got := readIndex(t, queued); len(got) != 1 {
		t.Errorf("the write did not land where it was queued: %d entries in %s", len(got), queued)
	} else if _, ok := got["queued"]; !ok {
		t.Errorf("wrong contents where it was queued: %v", got)
	}
	if got := readIndex(t, later); got != nil {
		t.Errorf("a previous save wrote %d entries into a directory it was never told about — "+
			"this is the CI failure, in one assertion", len(got))
	}
}

// TestADebouncedSaveDoesNotOverwriteTheNextTestsFile is the same fault stated
// the way it was actually met: the later directory already has a file, and the
// pending write must not touch it.
func TestADebouncedSaveDoesNotOverwriteTheNextTestsFile(t *testing.T) {
	quickDebounce(t)

	first, second := t.TempDir(), t.TempDir()
	t.Setenv("HOME", first)

	indexMutex.Lock()
	index = map[string]*IndexEntry{
		"leaked-a": {ID: "leaked-a", Type: "news", Title: "Public bitcoin rally"},
		"leaked-b": {ID: "leaked-b", Type: "news", Title: "Another"},
		"leaked-c": {ID: "leaked-c", Type: "news", Title: "Third"},
	}
	indexMutex.Unlock()

	done := start(t)

	// The next test: its own home, its own two-entry index.json — exactly what
	// TestSQLiteMigration writes before it migrates.
	t.Setenv("HOME", second)
	dir := filepath.Join(second, ".mu", "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := []byte(`{"test1":{"id":"test1","type":"news","title":"Test Article"},` +
		`"test2":{"id":"test2","type":"video","title":"Test Video"}}`)
	if err := os.WriteFile(filepath.Join(dir, "index.json"), mine, 0o644); err != nil {
		t.Fatal(err)
	}
	<-done

	got := readIndex(t, second)
	if len(got) != 2 {
		t.Errorf("Expected 2 entries, got %d — the failure this reproduces", len(got))
	}
	if _, ok := got["test1"]; !ok {
		t.Error("Entry not found — the second half of the same failure")
	}
}

// TestTheDebounceStillBatches — the wait is the reason this is asynchronous at
// all, and a fix that wrote immediately would trade one bug for a disk write
// per indexed item.
func TestTheDebounceStillBatches(t *testing.T) {
	quickDebounce(t)
	t.Setenv("HOME", t.TempDir())

	indexMutex.Lock()
	index = map[string]*IndexEntry{"one": {ID: "one", Type: "news", Title: "One"}}
	indexMutex.Unlock()

	done := start(t)

	// A second call while the first is pending is absorbed rather than queued.
	time.Sleep(5 * time.Millisecond)
	saveIndex() // returns at once; if it waited, this test would take twice as long

	select {
	case <-done:
		t.Error("the save did not wait, so a burst of updates is a burst of writes")
	default:
	}
	<-done
}
