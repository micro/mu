package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mu/internal/thread"
)

// A deleted conversation stays deleted across a restart.
//
// It did not. Deleting one removed the record of it and left the workflow runs
// it was made of, and adoptAll — which exists to carry chains written before
// the record existed across into it — read those runs at the next start-up and
// put the conversation back. Absence was the only thing adoption had to go on,
// so it could not tell "never adopted" from "adopted, then deleted by the
// person who owned it".
//
// The fix is not a tombstone: it is that deleting a conversation deletes the
// runs that made it up, because a run belongs to the conversation it was part
// of. Then there is nothing left to adopt.
func TestADeletedConversationDoesNotComeBackAtStartup(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	const who = "adopt-acc"
	thread.Deleted = ForgetConversation
	t.Cleanup(func() { thread.Deleted = nil })

	// Two turns of a conversation, as the web wrote them before the record
	// existed: a chain of runs linked by ParentID and nothing else.
	root := &Flow{ID: newFlowID(), AccountID: who, Prompt: "what is the weather",
		Answer: "Sunny.", Status: "done", CreatedAt: time.Now().Add(-time.Hour)}
	next := &Flow{ID: newFlowID(), AccountID: who, ParentID: root.ID,
		Prompt: "and tomorrow", Answer: "Rain.", Status: "done", CreatedAt: time.Now()}
	for _, f := range []*Flow{root, next} {
		if err := saveFlow(f); err != nil {
			t.Fatal(err)
		}
	}

	// Start-up carries it into the record.
	adoptAll()
	th := thread.Find(who, thread.WebClient, root.ID)
	if th == nil {
		t.Fatal("adoption did not carry the chain into the record")
	}

	// The person deletes it.
	thread.Delete(who, th.ID)
	if got := thread.Find(who, thread.WebClient, root.ID); got != nil {
		t.Fatal("delete left the conversation in the record")
	}

	// And a restart does not put it back.
	adoptAll()
	if got := thread.Find(who, thread.WebClient, root.ID); got != nil {
		t.Error("the conversation came back after a restart")
	}
	if got := len(sessionChain(who, root.ID)); got != 0 {
		t.Errorf("%d runs survived the conversation they belonged to", got)
	}
}
