package home

// Who is on this instance, under the box.
//
// Two things must hold and they pull against each other. The strip draws when
// you are the only one here — otherwise nobody on a one-person instance ever
// sees it, and cannot tell it works — and it must not, in that state, say
// anything about being alone. "Just you here" over a link to the chat shipped
// once and came out: it told somebody they were alone and then invited them to
// go and talk about it. A lit dot beside your own name states the same fact and
// does not editorialise.
//
// Driven through hereStrip rather than hereHTML. roster reads every account in
// the package and everything presence has heard of, which in a test binary is
// whatever every other test in this package left behind — so nothing that goes
// through it can be asked "and when I am the only one here". See hereStrip.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// Alone: your own name, lit, and nothing else.
//
// The negative half is the point. No count, no sentence about being alone, and
// no way through to a chat with nobody in it.
func TestHereDrawsWhenYouAreAlone(t *testing.T) {
	const who = "hereonly"
	got := hereStrip([]person{{id: who, online: true}}, who)

	if got == "" {
		t.Fatal("alone, the strip draws nothing — nobody on a one-person " +
			"instance ever sees it, and cannot tell it works")
	}
	if !strings.Contains(got, `href="/@`+who+`"`) {
		t.Errorf("your own name is not there, or is not a way to your page:\n%s", got)
	}
	if !strings.Contains(got, "here-on") {
		t.Errorf("you are on the instance and your dot is not lit:\n%s", got)
	}
	if strings.Contains(got, "muChatPanel") {
		t.Error("alone, the strip offers a way through to a chat with nobody in it")
	}
	for _, banned := range []string{"Just you", "just you", "alone", "Nobody", "nobody"} {
		if strings.Contains(got, banned) {
			t.Errorf("the strip says %q — it states who is here, it does not "+
				"narrate being alone:\n%s", banned, got)
		}
	}

	// Signed out it says nothing at all. The strip names accounts, and on an
	// instance that takes signups that would be a directory of its users,
	// published, to anybody who loads the front page.
	if got := hereHTML(""); got != "" {
		t.Errorf("a signed-out reader is given the roster: %q", got)
	}
}

// Somebody else here: named, lit, and a way to them.
func TestHereNamesTheOthersPresent(t *testing.T) {
	got := hereStrip([]person{
		{id: "mine", online: true},
		{id: "them", online: true},
	}, "mine")

	for _, id := range []string{"mine", "them"} {
		if !strings.Contains(got, `href="/@`+id+`"`) {
			t.Errorf("@%s is not in the strip:\n%s", id, got)
		}
	}
	// The control, not a link. It opens the panel on this page rather than
	// navigating to one — see people.PanelHTML — so the assertion is on what it
	// does, which is the part that would break silently if the panel went away.
	if !strings.Contains(got, "muChatPanel(true)") {
		t.Errorf("somebody else is here and there is no way to them:\n%s", got)
	}
}

// The live roster holds only people who are here, and only people.
//
// The one test that goes through roster rather than hereStrip. It reads the
// real package maps, so it asserts properties of whatever it finds rather than
// naming anybody: this binary's account store holds every account every other
// test in the package created.
//
// Last in this file, and it has to stay last. auth.UpdatePresence writes into a
// package map with a three minute window and nothing takes a name back out, so
// once this has run, every render after it in this binary says so.
func TestZZTheRosterIsWhoIsHere(t *testing.T) {
	const who = "hereroster"
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck
	auth.UpdatePresence(who)

	live := map[string]bool{}
	for _, id := range auth.OnlineUsers() {
		live[id] = true
	}

	people := roster()
	if len(people) == 0 {
		t.Fatal("somebody was just marked present and the roster is empty")
	}
	found := false
	for _, p := range people {
		if p.id == who {
			found = true
		}
		if !p.online {
			t.Errorf("@%s is on the roster and is not marked present — the "+
				"strip lists who is here, so every entry is lit", p.id)
		}
		if !live[p.id] {
			t.Errorf("@%s is on the roster and is not online; it listed every "+
				"account once, which on a real instance is a wall of strangers "+
				"under a heading saying HERE", p.id)
		}
		if auth.IsAgent(p.id) {
			t.Errorf("@%s is a program, not a person — it is the instance "+
				"itself, permanently present, and it is on the strip", p.id)
		}
	}
	if !found {
		t.Errorf("@%s was marked present and is not on the roster", who)
	}
}
