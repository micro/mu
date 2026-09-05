package home

// One axis, and it is the middle one now.
//
// This page was left-aligned against a 760px edge, and the reason was real:
// six things of different weights centred against each other have a ragged edge
// on both sides, so nothing lines up and they read as fragments. That is what
// happens when a page has six things on it.
//
// It has four: a name, a box, a date, two sentences. Hung off a left edge they
// are three items pinned to the side of an empty screen, and the axis that was
// holding a document together has no document to hold. Centred and narrow, they
// read as one object — which is what a page with almost nothing on it needs to
// do to look composed rather than unfinished.
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

	// And it is narrower than the shared measure, deliberately.
	//
	// 760 is what a page with a document on it uses, and there is no document
	// here — a name, a box, a date and two sentences. A short block centred in
	// a wide column has the ragged-edge problem the old left axis was avoiding,
	// so the fix is a measure the eye takes in without tracking.
	if !strings.Contains(src, ".lwrap{padding:0;max-width:560px") {
		t.Error("the landing is at the document measure, which is too wide for\n" +
			"four centred elements")
	}
	if !strings.Contains(src, "text-align:center}") {
		t.Error("the landing is not centred, so the name sits above a box that\n" +
			"does not line up with it")
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

// The logged-out human front door is Micro. Mu remains the runtime and the
// installed app name, so this deliberately pins only the landing surface.
func TestTheLoggedOutFrontDoorIsMicro(t *testing.T) {
	src, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `Title:       "Micro",`) {
		t.Error("the logged-out page title is not Micro")
	}
	body := indexBody()
	if !strings.Contains(body, `<div class="lbrand">Micro</div>`) {
		t.Error("the logged-out wordmark is not Micro")
	}
	if !strings.Contains(body, `<div class="lwhat">A personal assistant</div>`) {
		t.Error("the logged-out caption changed")
	}
}
