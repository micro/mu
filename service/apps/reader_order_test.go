package apps

// Whose apps go at the top of the directory.
//
// Officials led, on the argument that a first-time reader is asking what this
// thing can do and a calculator somebody uploaded is not the answer. That is
// right for a first-time reader and wrong every time after: the apps you wrote
// were filed under "From the community", below a fixed list you had already
// read, on your own server. The first visit is one visit.
//
// So the reader decides. A stranger gets the catalogue and the person who lives
// here gets their own things first — both readings of "what goes at the top"
// are correct for different readers, and the viewer is the only thing that can
// tell them apart.

import (
	"testing"
	"time"
)

// order is the slugs in the order they would be drawn.
func order(list []*App) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Slug
	}
	return out
}

func sample() []*App {
	now := time.Now()
	return []*App{
		{Slug: "builtin", Official: true, Order: 1},
		{Slug: "theirs", AuthorID: "someone", Installs: 99, CreatedAt: now},
		{Slug: "mine", AuthorID: "me", CreatedAt: now},
	}
}

// Signed in: yours, then everybody else's, then the ones that ship with the
// instance.
func TestYourOwnAppsLeadTheDirectory(t *testing.T) {
	list := sample()
	sortForReader(list, "me")

	if got := order(list); got[0] != "mine" {
		t.Errorf("the directory reads %v — your own app is not first on your "+
			"own server", got)
	}
	if got := order(list); got[len(got)-1] != "builtin" {
		t.Errorf("the directory reads %v — the built-ins still lead, which is "+
			"the fixed list you have already read", got)
	}
}

// Signed out, and nothing about the order changes.
//
// The JSON listing returns the same bytes to every caller, so an ordering that
// varied by who asked would make one response wrong for the next one. This is
// also the first-visit case the old order was built for, and it keeps it.
func TestAReaderlessListingKeepsTheCatalogueOrder(t *testing.T) {
	list := sample()
	sortForReader(list, "")

	if got := order(list); got[0] != "builtin" {
		t.Errorf("with no reader the directory reads %v — a stranger should "+
			"get the catalogue, which is what the instance ships", got)
	}

	// And sortForDirectory is that case by definition, so the two cannot drift.
	direct := sample()
	sortForDirectory(direct)
	a, b := order(direct), order(list)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sortForDirectory gives %v and sortForReader with no "+
				"viewer gives %v; they are meant to be the same call", a, b)
		}
	}
}

// Somebody else's account does not make their apps yours.
//
// The rule is authorship, not "not official" — without that, one person's page
// would promote every app on a shared instance and the section headed Yours
// would name apps you have never seen.
func TestAnotherReaderGetsTheirOwnAppsFirst(t *testing.T) {
	list := sample()
	sortForReader(list, "someone")

	if got := order(list); got[0] != "theirs" {
		t.Errorf("the directory reads %v for @someone — they should lead with "+
			"their own", got)
	}
	if got := order(list); got[1] != "mine" {
		t.Errorf("the directory reads %v — somebody else's app is being "+
			"promoted as if it were theirs", got)
	}
}
