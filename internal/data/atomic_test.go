package data

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome points the store at a scratch $HOME for the duration of a test.
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// Credentials, sessions, tokens, passkeys and wallet state all go through
// SaveJSON. None of it may land world-readable.
func TestSavedFilesAreOwnerOnly(t *testing.T) {
	withTempHome(t)

	if err := SaveJSON("accounts.json", map[string]string{"a": "b"}); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	if err := SaveFile("secret.txt", "token"); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}

	for _, key := range []string{"accounts.json", "secret.txt"} {
		p, err := dataPath(key)
		if err != nil {
			t.Fatalf("dataPath(%q): %v", key, err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %q: %v", key, err)
		}
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Errorf("%s mode = %04o, want 0600 (must not be group/world readable)", key, perm)
		}
	}
}

// A failed write must leave the previous contents intact rather than
// truncating the file — the whole point of writing via a temp file + rename.
func TestSaveJSONPreservesPreviousContentsOnMarshalFailure(t *testing.T) {
	withTempHome(t)

	if err := SaveJSON("blob.json", map[string]string{"keep": "me"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// channels cannot be marshalled — SaveJSON must fail before touching the file
	if err := SaveJSON("blob.json", map[string]interface{}{"bad": make(chan int)}); err == nil {
		t.Fatal("expected marshal error")
	}

	var got map[string]string
	if err := LoadJSON("blob.json", &got); err != nil {
		t.Fatalf("LoadJSON after failed write: %v", err)
	}
	if got["keep"] != "me" {
		t.Fatalf("previous contents lost: %+v", got)
	}
}

// Round-tripping must still work, and no stray temp files may be left behind.
func TestSaveJSONRoundTripLeavesNoTempFiles(t *testing.T) {
	withTempHome(t)

	want := map[string]string{"hello": "world"}
	if err := SaveJSON("round.json", want); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	var got map[string]string
	if err := LoadJSON("round.json", &got); err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if got["hello"] != "world" {
		t.Fatalf("round trip = %+v", got)
	}

	p, _ := dataPath("round.json")
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}

	// Stored bytes are valid JSON (not a partial write).
	b, err := LoadFile("round.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if !json.Valid(b) {
		t.Errorf("stored bytes are not valid JSON: %q", b)
	}
}
