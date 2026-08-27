package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This service keeps its own record and does not reach into anybody else's.
//
// service/mail has never imported internal/thread: it owns mail, and agent/mail
// writes the prose copy above it. Chat had that backwards for a day — stanzas
// went straight into the record, which meant a message between two people, with
// no agent anywhere near it, turned up in an inbox nobody addressed.
//
// The rule is checked rather than remembered because the failure is invisible:
// everything still works, a page just quietly shows something that is not its
// business.
func TestChatDoesNotWriteToTheRecord(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"mu/internal/thread"`) {
			found = append(found, f)
		}
	}
	if len(found) > 0 {
		t.Errorf("%v import internal/thread — a delivery mechanism keeps its own "+
			"record, and the prose copy an agent remembers is written above it "+
			"by agent/chat, the way agent/mail does it for mail", found)
	}
}

// A conversation between two people is not the agent's business.
//
// It follows from the service owning its record rather than from a filter
// applied afterwards: nothing writes it to internal/thread, so nothing has to
// remember to leave it out of /inbox.
func TestTwoPeopleTalkingStaysOutOfTheRecord(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	a, atok := accountWithToken(t, "chatprivate")
	accountWithToken(t, "chatother")

	c := dial(t)
	defer c.Close()
	c.handshake(t, a.ID, atok)

	c.write(`<message type='chat' to='chatother@example.test'>` +
		`<body>nothing to do with any agent</body></message>`)
	c.silence(t)

	conv := xmppRoom("chatprivate@example.test", "chatother@example.test")
	if n := len(Conversation("chatprivate", conv, 10)); n != 1 {
		t.Fatalf("the service kept %d copies of it, want 1", n)
	}
	// And the record above knows nothing about it. Asserted through the same
	// door /inbox reads, so this fails if a copy ever starts being written.
	if got := recorded(t, "chatprivate"); got != 0 {
		t.Errorf("%d messages reached the record from a conversation no agent "+
			"was in — that is what puts private chat in somebody's inbox", got)
	}
}
