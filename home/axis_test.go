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
	if strings.Contains(src, `class="btag"`) {
		t.Error("the wordmark has a description under it again. That is our copy\n" +
			"for our product, and every instance anybody deploys serves this\n" +
			"page — /about is where it belongs, and it is theirs to change")
	}

	// It does not say what this is, and that is the decision rather than an
	// omission.
	//
	// "A personal assistant" was under the name for a while. It reads well on
	// micro.mu and is our description of our product, served by every instance
	// anybody deploys — a stranger's server explaining itself in our words is
	// the same fault as our name in their header, one size down. See /about.

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
	// The name, not the address. micro.mu is "Micro": a wordmark is a name and
	// a hostname is an address, and the TLD is the part that makes it the
	// second one.
	for _, c := range []struct{ domain, want string }{
		{"micro.mu", "Micro"},
		{"assistant.example.com", "Assistant"},
		{"www.example.com", "Example"}, // www names nothing
		{"example.test:8080", "Example"},
		{"https://micro.mu/", "Micro"},
	} {
		t.Setenv("MU_DOMAIN", c.domain)
		if got := brand(); got != c.want {
			t.Errorf("brand() for %q = %q, want %q", c.domain, got, c.want)
		}
	}
	// And a machine that has not been told its own name — a development box —
	// falls back rather than showing an empty corner.
	t.Setenv("MU_DOMAIN", "")
	if got := brand(); got != "Mu" {
		t.Errorf("an unconfigured instance calls itself %q", got)
	}
	// No description under it. "A personal assistant" is our copy for our
	// product, and every instance anybody deploys serves this page — a
	// stranger's server explaining itself in our words is the same fault as our
	// name in their header, one size down. /about is where that belongs, and it
	// is theirs to change.
	t.Setenv("MU_DOMAIN", "example.test")
	if strings.Contains(brand(), "personal assistant") {
		t.Errorf("the wordmark carries our description of our product: %q", brand())
	}
	// A domain is a value an operator sets, and it lands in markup.
	t.Setenv("MU_DOMAIN", `<script>x</script>`)
	if strings.Contains(brand(), "<script>") {
		t.Errorf("the domain went onto the page as markup: %q", brand())
	}
}
