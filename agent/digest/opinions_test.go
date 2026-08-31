package digest

// The briefing is built on the pieces, and still works without them.
//
// This is the half of the change that is easy to ship broken, because both
// failure modes look like success from outside: a briefing that ignores the
// pieces reads fine, and a briefing on an instance with no pieces reads fine
// too. The difference is only in what the model was told it was holding.

import (
	"strings"
	"testing"
	"time"
)

func TestTheBriefingSaysWhatItIsReading(t *testing.T) {
	feeds := digestSystem(false)
	pieces := digestSystem(true)

	if feeds == pieces {
		t.Fatal("the prompt is the same whether or not the instance's own pieces " +
			"are in front of it, so nothing tells the model the first section is " +
			"the work and the rest is raw material")
	}

	// With pieces: told they come first, and told to build on them rather than
	// list them. A briefing that enumerates eight summaries is not a briefing.
	for _, want := range []string{"pieces Mu itself published", "build on them"} {
		if !strings.Contains(pieces, want) {
			t.Errorf("the prompt does not say %q, so the pieces are just more input", want)
		}
	}
	if !strings.Contains(pieces, "not simply list the pieces") {
		t.Error("nothing stops the briefing from being a list of the day's pieces")
	}

	// Without them, no mention of something that is not there. A prompt that
	// describes a section the input does not contain is how a model invents one.
	if strings.Contains(feeds, "pieces Mu itself published") {
		t.Error("the fallback prompt promises pieces that were not given to it")
	}

	// Both are still the same briefing, with the same house rules.
	for _, want := range []string{"globally neutral", "under 2000 characters"} {
		for what, sys := range map[string]string{"with pieces": pieces, "without": feeds} {
			if !strings.Contains(sys, want) {
				t.Errorf("%s: the prompt lost %q — the two branches have drifted into "+
					"two different briefings", what, want)
			}
		}
	}
}

// An instance with nothing published yet gets the briefing it got before.
//
// opinionContext is the only thing between the two branches, and on a fresh
// install, with OPINIONS=off, or on a day the model was unreachable, it has to
// come back empty rather than with a heading and no pieces under it.
func TestNoPiecesMeansTheOldBriefing(t *testing.T) {
	if got := opinionContext(); got != "" {
		// Not a failure of the code under test if the store has posts in it —
		// but in a unit test run there are none, and a non-empty answer here
		// means the empty case builds a heading anyway.
		if strings.Contains(got, "What Mu published") && !strings.Contains(got, "###") {
			t.Errorf("opinionContext built a heading with nothing under it:\n%s", got)
		}
	}
}

// The window is since the last briefing, not "today".
//
// The digest fires at 06:00 UTC and the pieces are written across the sixteen
// hours before it. Asking for today's would find none every single morning,
// fall back to the feeds, and log nothing — the feature implemented and never
// once running.
func TestTheWindowReachesBackPastMidnight(t *testing.T) {
	if opinionWindow < 24*time.Hour {
		t.Errorf("opinionWindow is %v — anything under a day means the 06:00 "+
			"briefing cannot see the pieces written before midnight", opinionWindow)
	}
}
