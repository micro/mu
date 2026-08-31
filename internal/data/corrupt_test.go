package data

// A store that will not load must not be overwritten.
//
// Every store here treats a load error as "no data" and starts empty — and the
// next write saves that empty state over the file. One unparseable byte was
// total, silent, permanent loss, made permanent by the very next message. It
// was one save away, on every store, all the time.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	badMu.Lock()
	bad = map[string]string{}
	badMu.Unlock()
}

func TestAnUnreadableStoreIsMovedAsideNotOverwritten(t *testing.T) {
	tempHome(t)

	if err := SaveJSON("things.json", map[string]string{"a": "one", "b": "two"}); err != nil {
		t.Fatal(err)
	}
	file, err := dataPath("things.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"a": "one", tru`), 0600); err != nil {
		t.Fatal(err)
	}

	var got map[string]string
	if err := LoadJSON("things.json", &got); err == nil {
		t.Fatal("a truncated file loaded without complaint")
	}

	// The file is gone from under its own name, so the empty state a caller
	// now holds cannot be written over it.
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("the unreadable file is still in place, so the next save destroys it")
	}
	aside, ok := Quarantined()["things.json"]
	if !ok {
		t.Fatal("nothing recorded the failure, so a status page cannot report it")
	}
	b, err := os.ReadFile(aside)
	if err != nil {
		t.Fatalf("the set-aside file is not readable: %v", err)
	}
	if !strings.Contains(string(b), `"one"`) {
		t.Error("the set-aside file does not contain what was there")
	}

	// And the store carries on: a later save works and does not resurrect the
	// bad file.
	if err := SaveJSON("things.json", map[string]string{"c": "three"}); err != nil {
		t.Fatal(err)
	}
	var after map[string]string
	if err := LoadJSON("things.json", &after); err != nil {
		t.Fatalf("the store did not recover: %v", err)
	}
	if after["c"] != "three" {
		t.Error("the instance cannot write after a quarantine")
	}
}

// A missing file is not a failure.
func TestAStoreThatHasNeverBeenWrittenIsNotQuarantined(t *testing.T) {
	tempHome(t)

	var got map[string]string
	if err := LoadJSON("never-written.json", &got); err == nil {
		t.Fatal("loading a file that does not exist returned no error")
	}
	if len(Quarantined()) != 0 {
		t.Error("a store that has never been written was reported as corrupt, which " +
			"makes the report worthless on every first run")
	}
}

// A write that loses most of a store keeps a copy.
//
// The wallet was destroyed this way: two components each loaded the same file
// into a map of their own and each saved its own view back over the other's.
func TestAWriteThatLosesMostOfAStoreKeepsACopy(t *testing.T) {
	tempHome(t)

	full := map[string]string{}
	for i := 0; i < 200; i++ {
		full[string(rune('a'+i%26))+strings.Repeat("x", 20)+string(rune(i))] = strings.Repeat("y", 40)
	}
	if err := SaveJSON("wallets.json", full); err != nil {
		t.Fatal(err)
	}
	file, _ := dataPath("wallets.json")
	before, _ := os.Stat(file)

	// A caller with a stale view saves what little it knows.
	if err := SaveJSON("wallets.json", map[string]string{"one": "left"}); err != nil {
		t.Fatalf("SaveJSON rejected a store shrink after preserving it: %v", err)
	}
	var current map[string]string
	if err := LoadJSON("wallets.json", &current); err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current["one"] != "left" {
		t.Errorf("accepted write left store as %#v, want only the current value", current)
	}

	prev := file + ".prev"
	b, err := os.ReadFile(prev)
	if err != nil {
		t.Fatalf("a write that lost %d bytes kept no copy: %v", before.Size(), err)
	}
	var recovered map[string]string
	if err := json.Unmarshal(b, &recovered); err != nil {
		t.Fatalf("the kept copy does not parse: %v", err)
	}
	if len(recovered) != len(full) {
		t.Errorf("the kept copy has %d entries, want %d", len(recovered), len(full))
	}
}

// An ordinary write does not.
func TestAnOrdinaryWriteKeepsNoCopy(t *testing.T) {
	tempHome(t)

	m := map[string]string{}
	for i := 0; i < 200; i++ {
		m[strings.Repeat("k", 10)+string(rune(i))] = strings.Repeat("v", 40)
	}
	if err := SaveJSON("notes.json", m); err != nil {
		t.Fatal(err)
	}
	// One entry removed is not an accident.
	for k := range m {
		delete(m, k)
		break
	}
	if err := SaveJSON("notes.json", m); err != nil {
		t.Fatal(err)
	}
	file, _ := dataPath("notes.json")
	if _, err := os.Stat(file + ".prev"); err == nil {
		t.Error("an ordinary edit kept a copy — if every write does this, the copies " +
			"are noise and nobody looks at them")
	}
	_ = filepath.Base(file)
}
