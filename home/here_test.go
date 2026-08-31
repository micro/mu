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
	"time"

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
	if strings.Contains(got, `href="/chat"`) {
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
//
// And the ones who are not are still listed. Online-only drew nothing almost
// all the time — the usual number of other people on a personal instance at any
// given second is zero — and a strip that is empty on the day you look at it
// teaches you it is broken.
func TestHereListsEveryoneAndLightsThePresent(t *testing.T) {
	people := []person{
		{id: "mine", online: true},
		{id: "them", online: true},
		{id: "away", seen: time.Now().Add(-48 * time.Hour), known: true},
		{id: "never"},
	}
	got := hereStrip(people, "mine")

	for _, id := range []string{"mine", "them", "away", "never"} {
		if !strings.Contains(got, `href="/@`+id+`"`) {
			t.Errorf("@%s is not in the strip:\n%s", id, got)
		}
	}
	if !strings.Contains(got, `href="/chat"`) {
		t.Errorf("somebody else is here and there is no way to them:\n%s", got)
	}

	// Lit is a property of being present, not of being listed. Without that
	// the strip says everybody is always here, which is worse than saying
	// nothing: it is the one fact it exists to carry, wrong.
	if n := strings.Count(got, "here-on"); n != 2 {
		t.Errorf("%d names are lit, want 2:\n%s", n, got)
	}

	// How long ago, for the ones who are not here. Not for the ones who are —
	// the lit dot says it, and "1 min ago" beside a live dot is the same fact
	// twice. Not for an account presence has never seen either: after a restart
	// that is everybody, and a strip of names each carrying the same wrong time
	// is worse than a strip of names.
	if !strings.Contains(got, "here-when") {
		t.Errorf("nobody carries when they were last here:\n%s", got)
	}
	if n := strings.Count(got, "here-when"); n != 1 {
		t.Errorf("%d names carry a time, want 1 — only @away has one that is "+
			"both known and in the past:\n%s", n, got)
	}
}

// A stale last-seen is dropped rather than printed.
//
// A year against a name is not "when they were here", it is a fact about an
// abandoned account. The name still draws — they are on the instance and that
// is the point — without a number that makes the strip look like a graveyard.
func TestHereDropsAStaleLastSeen(t *testing.T) {
	got := hereStrip([]person{
		{id: "ancient", seen: time.Now().Add(-2 * 365 * 24 * time.Hour), known: true},
	}, "viewer")

	if !strings.Contains(got, `href="/@ancient"`) {
		t.Errorf("a long-absent account is dropped from the strip entirely:\n%s", got)
	}
	if strings.Contains(got, "here-when") {
		t.Errorf("a two-year-old last-seen is printed against a name:\n%s", got)
	}
}

// The live sort: whoever is present leads.
//
// The one test here that goes through roster, and all it checks is the
// ordering rule — a strip that sorted by name would bury the people who are
// actually here behind whoever happens to sort first. It reads the real maps,
// so it asserts a relationship between entries rather than naming any: this
// binary's account store holds whatever every other test in the package made.
//
// Last in this file, and it has to stay last. auth.UpdatePresence writes into
// a package map with a three minute window and nothing takes a name back out,
// so once this has run, every render after it in this binary says so.
func TestZZTheOnesPresentComeFirst(t *testing.T) {
	const who = "hereroster"
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck
	auth.UpdatePresence(who)

	people := roster()
	if len(people) < 2 {
		t.Skip("one account on this instance, so there is no order to check")
	}
	seenOffline := false
	for _, p := range people {
		if !p.online {
			seenOffline = true
			continue
		}
		if seenOffline {
			t.Fatalf("@%s is here and is listed after somebody who is not", p.id)
		}
	}
	if !people[0].online {
		t.Error("nobody leads the strip, yet somebody was just marked present")
	}
}
