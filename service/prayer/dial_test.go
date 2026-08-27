package prayer

// The qibla dial, as a thing you aim.
//
// Reported: "turn until Q is with the marker at the top… there's no marker."
// Both halves of that were true, and they were separate faults.
//
// The marker existed in the markup and could never be drawn: it carried
// .d-none, which is display:none !important, and the code revealed it by
// setting an inline display to the empty string — which removes the inline
// declaration and leaves the !important rule standing. Built, shipped,
// invisible.
//
// And even with it drawn the instruction could not be followed, because Q was
// placed at the needle's own angle. Q and the needle were one object, so
// "bring Q to the marker" only ever meant "point the arrow up", and there was
// nothing to line anything up against. The fix is the shape the reporter
// described: Q pinned to the target, the arrow the only thing that moves.
//
// This reads the script because that is where the behaviour is. A browser
// drove the real page to confirm all of it — needle at 0° and green when
// facing 119° from London, marker visible, Q fixed at the top through every
// heading — but a screenshot is not a regression test.

import (
	"strings"
	"testing"
)

func dialScript(t *testing.T) string {
	t.Helper()
	s := prayerTimesHTML()
	if !strings.Contains(s, "qibla-index") {
		t.Fatal("the dial has no target element at all")
	}
	return s
}

// The marker is revealed by dropping the class that hides it.
func TestTheMarkerCanActuallyBeShown(t *testing.T) {
	s := dialScript(t)

	if !strings.Contains(s, "classList.remove('d-none')") {
		t.Error("nothing removes .d-none from the target, so it stays hidden — " +
			"that class is display:none !important and an inline style does not " +
			"beat it")
	}
	if strings.Contains(s, "idx.style.display=''") {
		t.Error("the target is still revealed by clearing an inline style, which " +
			"an !important rule wins")
	}
}

// Q is the fixed target; the needle is what moves.
func TestQIsTheTargetAndTheNeedlePointsAtIt(t *testing.T) {
	s := dialScript(t)

	if !strings.Contains(s, "function pinQ(){setMark('qibla-q',0,") {
		t.Error("Q is not pinned to the top, so it has no fixed place to be the " +
			"marker for")
	}
	// The old placement: Q at the needle's angle, which makes the two one
	// thing. The call, not the definition — placeMarks still exists and is
	// still right for the static dial, where Q is a rose label and not a
	// target.
	if strings.Contains(s, "placeMarks(qAngle,(360-smoothed)") {
		t.Error("Q still rides the needle, so lining them up is impossible — they " +
			"are never apart")
	}
	if !strings.Contains(s, "points at Q") {
		t.Error("the instruction no longer describes aiming the arrow at Q")
	}
	if strings.Contains(s, "reaches the marker at the top") {
		t.Error("the old instruction is still there, and it describes a dial that " +
			"no longer exists")
	}
}

// And it says when you have arrived. "Turn until…" with no arrival reads the
// same facing Mecca as facing away from it.
func TestItSaysWhenYouAreFacingTheQibla(t *testing.T) {
	s := dialScript(t)
	if !strings.Contains(s, "Facing the qibla.") {
		t.Error("nothing confirms alignment, so the instruction never resolves")
	}
}

// With no heading there is nothing to aim, so the target stays away and the
// dial is what it can honestly be: a north-up diagram of a bearing.
func TestWithNoHeadingItIsADiagramAndNotAnInstrument(t *testing.T) {
	s := dialScript(t)
	if !strings.Contains(s, `class="d-none"`) {
		t.Error("the target is drawn before any heading exists, which invites " +
			"somebody to turn until an arrow that cannot move reaches it")
	}
	if !strings.Contains(s, "placeMarks(d.qibla.bearing,0)") {
		t.Error("the static dial no longer puts N at the top and Q at the bearing")
	}
}
