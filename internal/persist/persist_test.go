package persist

import (
	"encoding/json"
	"mu/internal/dir"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoversWholePendingCommitBeforeRead(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writes := map[string][]byte{"source.json": []byte(`{"value":"new"}`), "outbox/log/1.json": []byte(`{"id":"1"}`)}
	if err := os.MkdirAll(dir.Data(), 0700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(writes)
	if err := os.WriteFile(filepath.Join(dir.Data(), pending), b, 0600); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after replacing the source but before replacing the outbox.
	if err := os.WriteFile(filepath.Join(dir.Data(), "source.json"), writes["source.json"], 0600); err != nil {
		t.Fatal(err)
	}
	for key, want := range writes {
		got, err := Read(key)
		if err != nil || string(got) != string(want) {
			t.Fatalf("%s: %s %v", key, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir.Data(), pending)); !os.IsNotExist(err) {
		t.Fatalf("pending commit remains: %v", err)
	}
}

func TestInvalidKeysCannotOverwriteJournalOrEscape(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"", ".", "../escape", "/tmp/escape", pending, "sub/../" + pending} {
		if err := Batch(map[string][]byte{key: []byte("bad")}); err == nil {
			t.Fatalf("accepted %q", key)
		}
	}
	if err := Batch(map[string][]byte{"ok.json": []byte("ok")}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedCommitBlocksStaleWritesUntilRestart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { mu.Lock(); poisoned = nil; mu.Unlock() })
	if err := Write("blocked", []byte("file")); err != nil {
		t.Fatal(err)
	}
	if err := Batch(map[string][]byte{"blocked/child": []byte("committed")}); err == nil {
		t.Fatal("expected failed recovery")
	}
	if err := Write("other", []byte("stale")); err == nil {
		t.Fatal("allowed write after uncertain commit")
	}
	// A repaired filesystem and process restart must recover the committed data.
	if err := os.Remove(filepath.Join(dir.Data(), "blocked")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	poisoned = nil
	mu.Unlock()
	got, err := Read("blocked/child")
	if err != nil || string(got) != "committed" {
		t.Fatalf("%s %v", got, err)
	}
}
