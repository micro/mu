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

	// The wordmark shares the block's measure and its edge. It is a sibling in
	// the shell, not a child, so it has to be given both rather than inherit
	// them — without that the page's largest element is the one thing not on the
	// axis.
	if !strings.Contains(src, ".index-page .brand{width:100%;max-width:760px;text-align:left;") {
		t.Error("the wordmark is not aligned to the block, so the biggest thing on\n" +
			"the page sits on a different edge from everything under it")
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
	if strings.Contains(indexBody(""), `class="mu-chat-centred"`) {
		t.Error("the doors and options rows are centred while everything else on\n" +
			"the page hangs off the left edge")
	}
}
