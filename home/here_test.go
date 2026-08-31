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
	"strconv"
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
	// And no count. "1 person" is longer than "@you", says less, and a summary
	// of a list of one is how a page ends up telling somebody they are by
	// themselves.
	if strings.Contains(got, "here-count") {
		t.Errorf("alone, the strip counts you:\n%s", got)
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

// Somebody else here: how many, a way in, and the names one click away.
//
// The strip listed every name every time. A row of handles is a thing you read
// once and never again — a wall on a busy instance, the same four names every
// day on a quiet one — and the number is the part that changes and the part
// somebody wants: is anyone about. Who they are is a question, and a question
// gets answered when it is asked.
func TestHereCountsTheOthersAndHidesTheNames(t *testing.T) {
	got := hereStrip([]person{
		{id: "mine", online: true},
		{id: "them", online: true},
		{id: "third", online: true},
	}, "mine")

	if !strings.Contains(got, ">3 people</button>") {
		t.Errorf("the strip does not say how many people are here:\n%s", got)
	}

	// The names are there — this is a disclosure, not a removal — and they are
	// not on the screen until somebody asks.
	for _, id := range []string{"mine", "them", "third"} {
		if !strings.Contains(got, `href="/@`+id+`"`) {
			t.Errorf("@%s cannot be reached from the strip at all:\n%s", id, got)
		}
	}
	who := strings.Index(got, `id="here-who"`)
	if who < 0 {
		t.Fatalf("there is no list for the count to open:\n%s", got)
	}
	end := strings.Index(got[who:], ">")
	if !strings.Contains(got[who:who+end], "hidden") {
		t.Errorf("the names are on the page before anybody asked for them — which "+
			"is the row of handles this replaced:\n%s", got)
	}
	if !strings.Contains(got, "muHereWho(this)") {
		t.Errorf("the count does not open the names, so it is a label:\n%s", got)
	}

	// The way in. A control, not a link: it opens the panel on this page rather
	// than navigating to one — see panelHTML — so the assertion is on what it
	// does, which is the part that would break silently if the panel went away.
	if !strings.Contains(got, "muChatPanel(true)") {
		t.Errorf("somebody else is here and there is no way to them:\n%s", got)
	}
	// And it is a bubble, not a sentence.
	if strings.Contains(got, "Open chat →") || strings.Contains(got, ">Open chat<") {
		t.Errorf("the way in is captioned — the shape is the word:\n%s", got)
	}
	if !strings.Contains(got, `aria-label="Open chat"`) {
		t.Errorf("the bubble has no label, so it is nothing to a screen reader:\n%s", got)
	}
}

// The count is everybody here. The cap is on the names.
//
// hereShown used to be applied in roster, which was right while the strip was
// the names and wrong the moment it became a number: on an instance with forty
// people present the count would have said twelve, and a number that is
// silently a cap is worse than a list that is visibly one.
func TestTheCountIsNotCappedByWhatIsShown(t *testing.T) {
	var present []person
	for i := 0; i < hereShown+7; i++ {
		present = append(present, person{id: "p" + strconv.Itoa(i), online: true})
	}
	got := hereStrip(present, "p0")

	if !strings.Contains(got, ">"+strconv.Itoa(hereShown+7)+" people</button>") {
		t.Errorf("the count is not %d — it is reporting how many names fit:\n%s",
			hereShown+7, got)
	}
	if !strings.Contains(got, "and 7 more") {
		t.Errorf("7 people are missing from the revealed names and nothing says so:\n%s", got)
	}
	if strings.Contains(got, `/@p`+strconv.Itoa(hereShown)+`"`) {
		t.Errorf("the names are not capped, so a crowd is a paragraph:\n%s", got)
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
