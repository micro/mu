package thread

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFailedFlushRemainsRetryable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	file := filepath.Join(dir, ".mu", "data", "threads.json")
	if err := os.MkdirAll(file, 0700); err != nil {
		t.Fatal(err)
	}
	th := Open("flush-retry", ChatClient, "source")
	Add(Message{Thread: th.ID, Account: "flush-retry", Text: "keep this", Ref: "reply-once"})
	if err := Flush(); err == nil {
		t.Fatal("failed write reported a durable conversation")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := Flush(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file)
	if err != nil || len(b) == 0 {
		t.Fatalf("failed flush discarded the retry: %v", err)
	}
}
