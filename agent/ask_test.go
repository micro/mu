package agent

// The surround is shared, and that is the point.
//
// Every client reached the same agent already. What each one wrote itself was
// the part around it — history, the run record, memory — and three of them kept
// history in a map in memory, keyed differently, lost on restart, invisible
// anywhere else. These pin the pieces that can be checked without a model.

import (
	"os"
	"strings"
	"testing"
)

// No client keeps a conversation of its own any more.
//
// This is the whole change, and it is the kind that half-happens: one client
// gets migrated and two keep their maps, which is worse than before because now
// there are two answers to where history lives.
func TestNoClientKeepsItsOwnHistory(t *testing.T) {
	for _, c := range []struct{ path, name string }{
		// mail and the Meta WhatsApp bot were here. They are
		// deleted — 2,100 lines and three third-party APIs carrying no traffic.
		// Mail is the client that is left and the one the product rests on.
		{"mail/mail.go", "mail"},
	} {
		b, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, gone := range []string{"histories", "getHistory", "addHistory"} {
			if strings.Contains(src, gone) {
				t.Errorf("%s still has %s — a second copy of history that restarts "+
					"empty and nothing else can read", c.name, gone)
			}
		}
		if !strings.Contains(src, "agent.Ask(") {
			t.Errorf("%s does not go through agent.Ask, so it has its own surround", c.name)
		}
	}
}

// Every client teaches memory, not just the web.
func TestEveryClientTeachesMemory(t *testing.T) {
	b, err := os.ReadFile("ask.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "extractMemory") {
		t.Error("Ask does not extract memory, so an agent still only learns from " +
			"whichever client happens to call it — which was the web, and nowhere else")
	}
}

// There was a test here that a client keys a conversation on the person and not
// the room, because a another client channel is many people talking and
// keying on the channel handed each of them the others' history as context.
//
// It has no subject any more. Those clients are deleted, and mail has no shared
// room: a thread is identified by the References chain it belongs to, which is
// per-conversation by construction. If a many-people client ever comes back, so
// does this.
