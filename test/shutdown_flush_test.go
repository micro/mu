package test

// What was said survives the process that heard it.
//
// internal/thread does not write on every message. save() sets a flag and a
// flusher does the work at most once a second, because writing the whole file
// per message with the lock held froze the UI while a few hundred conversations
// were adopted. That is the right trade, and it has one requirement: something
// has to flush when the process is going away.
//
// Nothing did. Flush's own comment said it was exported "for the two callers
// that cannot wait for the tick: a test that wants to know the file is on disk,
// and anything shutting down" — and the shutting-down caller had never been
// written. So every restart dropped up to a second of conversation, which on an
// instance that redeploys on a push means: ask a question, have the deploy land
// in that second, and your message is missing from the page you reload.
//
// Read from source rather than by running a server, because what is being held
// is that the call is on the shutdown path at all.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShutdownWritesTheConversationRecord(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(at(""), "internal/server/serve.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	i := strings.Index(src, "server.Shutdown(ctx)")
	if i < 0 {
		t.Fatal("the graceful shutdown has moved; this test needs repointing")
	}
	if !strings.Contains(src, "thread.Flush()") {
		t.Fatal("nothing flushes the conversation record when the process stops, " +
			"so a restart loses up to a second of what was said — and this " +
			"instance restarts on every deploy")
	}
	// After the drain, not before: an agent run finishing during shutdown
	// records its answer, and flushing first would write the file and leave
	// that answer in memory only.
	if strings.Index(src, "thread.Flush()") < i {
		t.Error("the record is flushed before in-flight requests have drained, so " +
			"an answer that lands during the drain is written to memory and " +
			"never to disk")
	}
}
