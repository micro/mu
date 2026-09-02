package home

// One axis.
//
// Everything on the front door was centred in a 640px column on a 1280px
// screen. Two failures compounded: two thirds of the viewport was empty so the
// container could only grow downward — every element added made the page taller
// rather than fuller — and six things of different weights centred against each
// other have a ragged edge on both sides, so nothing lines up with anything and
// they read as fragments rather than as one thing.
//
// Left, against a single edge, at the measure every other page uses. The block
// is still centred in the screen; its contents are not centred in the block.
//
// The alignment itself is measured in a browser — seven elements, one left edge
// — because CSS is what decides it. What is pinned here is the intent, so that
// a later change has to argue with it rather than drift past it.

import (
	"os"
	"strings"
	"testing"
)

func TestTheFrontDoorHangsOffOneEdge(t *testing.T) {
	b, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	// Nothing inside the block centres its own contents.
	for _, centred := range []string{
		".ltoday{margin:34px auto 0;max-width:580px;text-align:center}",
		".lbrief{text-align:center",
		".lcap{text-align:center",
	} {
		if strings.Contains(src, centred) {
			t.Errorf("%q centres a row against the others, which puts it on its own\n"+
				"axis and makes the block read as fragments", centred)
		}
	}

	// The wordmark is not on this page any more, and the axis survived it.
	//
	// It was the hero — 2.5rem, centred over the box — and it carried its own
	// rules for measure, edge, and how the tag sat beside it. Every one of
	// those existed to make a large centred name coexist with a left-aligned
	// document underneath. The name is a line in the top left now, the way it
	// is on any site, so there is nothing left to reconcile.
	//
	// The axis is unbroken because the header takes the same measure: .index-head
	// is max-width 760 with margin auto, so the name sits over the left edge of
	// the box rather than on an edge of its own. Measured, both land at x=260 in
	// a 1280 window.
	if strings.Contains(src, ".index-page .brand{") {
		t.Error("the front door is styling the wordmark again — it belongs to the\n" +
			"header now, and a page rule for it is the large-centred-name\n" +
			"arrangement coming back one property at a time")
	}
	if !strings.Contains(src, `class="btag"`) {
		t.Error("the name no longer says what it is, which is the one thing a\n" +
			"stranger reading a domain in a corner does not know")
	}

	// And it says what this is. A stranger arriving here has never been told:
	// the box, the row of services and the brief are all evidence, and none of
	// them is a sentence.
	if !strings.Contains(src, "a personal assistant") {
		t.Error("the front door never says what this is")
	}

	// And the block is the measure the rest of the product uses.
	if !strings.Contains(src, ".lwrap{padding:0;max-width:760px") {
		t.Error("the front door is not at the shared measure — see --measure in mu.css")
	}
}

// The chat component's furniture is left too, or the doors row centres itself
// under a left-aligned box.
func TestTheBoxsFurnitureIsNotCentredHere(t *testing.T) {
	// The class on the element, not the word: the component's own stylesheet
	// used to name it, and a substring match was true either way.
	if strings.Contains(indexBody(), `class="mu-chat-centred"`) {
		t.Error("the doors and options rows are centred while everything else on\n" +
			"the page hangs off the left edge")
	}
}

// The instance names itself, and a bare one still has a name.
//
// "Mu" is the software's name, not this server's. Mu is a thing you run, so on
// somebody else's box a wordmark reading Mu is our name on their front door —
// the same fault as the pricing copy that used to ship in every binary. What a
// visitor has arrived at is a domain, and the domain is what they will tell
// somebody else about.
func TestTheFrontDoorIsNamedAfterThisInstance(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	if got := brand(); !strings.Contains(got, "example.test") {
		t.Errorf("brand() = %q, want this instance's own domain", got)
	}
	if strings.Contains(brand(), ">Mu<") {
		t.Errorf("a configured instance still calls itself Mu: %q", brand())
	}
	// And a machine that has not been told its own name — a development box —
	// falls back rather than showing an empty corner.
	t.Setenv("MU_DOMAIN", "")
	if got := brand(); !strings.Contains(got, "Mu") {
		t.Errorf("an unconfigured instance has no name at all: %q", got)
	}
	// A domain is a value an operator sets, and it lands in markup.
	t.Setenv("MU_DOMAIN", `<script>x</script>`)
	if strings.Contains(brand(), "<script>") {
		t.Errorf("the domain went onto the page as markup: %q", brand())
	}
}
