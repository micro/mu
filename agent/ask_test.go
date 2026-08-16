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
		{"../client/discord/discord.go", "discord"},
		{"../client/telegram/telegram.go", "telegram"},
		{"../client/whatsapp/whatsapp.go", "whatsapp"},
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

// A thread key identifies a conversation, not a room.
//
// Keying on the channel alone made everything ever said in a shared channel one
// conversation, and handed each person the others' history as context. That is
// somebody else's mail, arriving as if the agent already knew them.
func TestAThreadKeySeparatesPeopleInASharedRoom(t *testing.T) {
	for _, c := range []struct{ path, name string }{
		{"../client/discord/discord.go", "discord"},
		{"../client/telegram/telegram.go", "telegram"},
	} {
		b, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "func threadKey(") {
			t.Errorf("%s has no threadKey, so a shared channel is one conversation "+
				"and everyone in it shares a history", c.name)
		}
	}
}
