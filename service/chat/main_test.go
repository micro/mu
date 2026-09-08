package chat

import (
	"os"
	"testing"
)

// Chat tests exercise durable transcripts and account setup. They must never
// write into the developer's live chat archive.
func TestMain(m *testing.M) {
	scratch, err := os.MkdirTemp("", "mu-chat-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", scratch)
	code := m.Run()
	os.RemoveAll(scratch)
	os.Exit(code)
}
