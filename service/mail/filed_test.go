package mail

// Delivering one message costs one message.
//
// It cost the whole store, and quadratically: rebuildInboxes discards every
// inbox and files every message again, and addMessageToInbox calls
// MessageUnlocked to find a new thread's root, which scans everything. n
// messages each scanning n.
//
//	messages stored    one delivery
//	            100          1.4 ms
//	          1,000         10.4 ms
//	          5,000         71.3 ms   ← 5x the messages, 6.9x the time
//
// All of it under the package write mutex, so the whole instance waited.
//
// The benchmark in bench_test.go is the measurement; this is the shape that
// produced it. A cost test would be flaky on a shared machine — this asserts
// the thing that was actually wrong, which is that delivery touched every
// message, and it fails the moment somebody puts rebuildInboxes back.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDeliveryFilesOneMessageRatherThanRebuilding(t *testing.T) {
	b, err := os.ReadFile("mail.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	// The two delivery paths: the local one and the inbound one. Both append a
	// message to the front of the store and then have to put it in an inbox.
	deliver := regexp.MustCompile(
		`addMessage\(msg\)\s*\n\s*(\w+)\(`)

	found := deliver.FindAllStringSubmatch(src, -1)
	if len(found) != 2 {
		t.Fatalf("found %d delivery sites, want 2 — this test is looking at the "+
			"wrong thing, or delivery has moved", len(found))
	}
	for _, m := range found {
		if m[1] != "fileMessage" {
			t.Errorf("delivery calls %s after appending one message; rebuilding the "+
				"whole store to add one is the quadratic cost this replaced —"+
				" see fileMessage", m[1])
		}
	}

	// And rebuildInboxes is still there for what genuinely changes everything:
	// loading the store, and deleting from it. Filing incrementally cannot
	// undo a removal.
	if !strings.Contains(src, "func rebuildInboxes()") {
		t.Error("rebuildInboxes has gone — a delete still has to rebuild, because " +
			"filing one message forward cannot take one out")
	}
}

// A message still reaches both inboxes, which is what filing incrementally
// must not lose.
func TestAFiledMessageReachesSenderAndRecipient(t *testing.T) {
	mutex.Lock()
	msgs, boxes := messages, inboxes
	setMessages(nil)
	inboxes = nil
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		setMessages(msgs)
		inboxes = boxes
		mutex.Unlock()
	})

	msg := &Message{
		ID: "filed1", ThreadID: "filed1",
		FromID: "filedfrom", ToID: "filedto",
		Subject: "hello", Body: "hello",
	}

	mutex.Lock()
	addMessage(msg)
	fileMessage(msg)
	got := len(inboxes)
	toBox := inboxes["filedto"]
	mutex.Unlock()

	if toBox == nil {
		t.Fatalf("the recipient has no inbox after delivery; %d inboxes exist", got)
	}
	if len(toBox.Threads) != 1 {
		t.Errorf("the recipient's inbox has %d threads, want the one just delivered",
			len(toBox.Threads))
	}
	if toBox.UnreadCount != 1 {
		t.Errorf("the delivered message is not unread for the recipient (count %d)",
			toBox.UnreadCount)
	}
}

// Spam is not filed, the same as it was not rebuilt.
func TestSpamIsNotFiled(t *testing.T) {
	mutex.Lock()
	msgs, boxes := messages, inboxes
	setMessages(nil)
	inboxes = nil
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		setMessages(msgs)
		inboxes = boxes
		mutex.Unlock()
	})

	junk := &Message{ID: "filedspam", ThreadID: "filedspam",
		FromID: "spammer", ToID: "filedto", Spam: true}

	mutex.Lock()
	addMessage(junk)
	fileMessage(junk)
	box := inboxes["filedto"]
	mutex.Unlock()

	if box != nil && len(box.Threads) > 0 {
		t.Error("spam was filed into the normal inbox, which is the one thing " +
			"rebuildInboxes skipped")
	}
}

// The store is only assigned through the two helpers that keep the lookups
// honest.
//
// byID and byHeaderID are built by setMessages and maintained by addMessage.
// An index kept up to date by remembering to keep it up to date is wrong on
// the path somebody forgot — and this package already had one of those before
// there was an index: removing a message marked spam skipped the inbox rebuild
// every other removal did.
//
// A stale entry is worse than the scan it replaced. A lookup that misses
// reports a message that exists as absent, which breaks threading and makes a
// delete claim there is nothing to delete. Fixtures count: a test that writes
// straight into the slice is testing a store the product cannot produce, and
// TestSearchScopedToAccount did exactly that and failed the moment search
// started going through the map.
func TestNothingAssignsTheStoreDirectly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// `messages = ` at the start of a statement. setMessages and addMessage are
	// where it is allowed, so lookups.go is the one file exempt.
	direct := regexp.MustCompile(`(?m)^\s*messages\s*(=[^=]|,[^=\n]*=[^=])`)

	for _, f := range files {
		if f == "lookups.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if !direct.MatchString(line) {
				continue
			}
			if s := strings.TrimSpace(line); strings.HasPrefix(s, "//") {
				continue
			}
			t.Errorf("%s:%d assigns the store directly:\n    %s\n"+
				"    Use setMessages or addMessage — see lookups.go — or byID and "+
				"byHeaderID stop matching what is stored.",
				f, i+1, strings.TrimSpace(line))
		}
	}
}

// And the lookups answer from the maps rather than by walking.
func TestFindingOneMessageDoesNotReadThemAll(t *testing.T) {
	b, err := os.ReadFile("mail.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	for _, fn := range []string{"func MessageUnlocked(", "func byMessageIDUnlocked("} {
		i := strings.Index(src, fn)
		if i < 0 {
			t.Fatalf("%s has gone", fn)
		}
		body := src[i:]
		if j := strings.Index(body, "\n}\n"); j > 0 {
			body = body[:j]
		}
		if strings.Contains(body, "range messages") {
			t.Errorf("%s still walks the whole store; it is called once per new "+
				"thread and once per inbound reply, holding the write mutex", fn)
		}
	}
}
