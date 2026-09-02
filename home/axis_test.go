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

// One name, on every surface.
//
// The wordmark derived a name from the hostname for a while — micro.mu reading
// as "Micro" — on the argument that Mu is what you run rather than what you
// arrived at, so our name on somebody else's front door is the same fault as
// the pricing copy that used to ship in every binary.
//
// The argument is real and the fix was in the wrong place. It changed one
// surface: the browser tab still said Mu, the manifest still said Mu, and the
// app still installed as Mu with a Mu icon. Four surfaces, two names, and
// nothing explaining the relation — worse than either answer on its own.
//
// So this pins the agreement rather than the string. Whatever the wordmark
// says, the title and the manifest say the same thing; a self-hosted instance
// that wants its own name is a setting that moves all three, not a hostname
// parsed into one of them.
func TestTheNameIsTheSameOnEverySurface(t *testing.T) {
	src, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, `Brand:    "Mu",`) {
		t.Error("the wordmark is not the instance's one name — if it is derived\n" +
			"from something, the title and the manifest have to be derived from\n" +
			"the same thing or the product has two names and explains neither")
	}
	if !strings.Contains(body, `Title:       "Mu",`) {
		t.Error("the page title and the wordmark disagree")
	}
	// And the manifest, which is what an installed app is called on a home
	// screen — the surface somebody looks at most and can change least.
	man, err := os.ReadFile("../internal/app/html/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(man), `"name": "Mu"`) {
		t.Error("the installed app is called something other than the wordmark")
	}
}
