package hazards

// Reading flood warnings.

import (
	"strings"
	"testing"
)

// The four levels are the whole point of the scale, and three of them are not
// the same instruction. A page that flattened them into "warning" would lose
// the only distinction anybody acts on differently.
func TestTheSeverityLevelsAreWordsNotNumbers(t *testing.T) {
	for level, want := range map[int]string{
		1: "danger to life",
		2: "act now",
		3: "be prepared",
	} {
		if !strings.Contains(severity[level], want) {
			t.Errorf("level %d reads %q, which does not say %q", level, severity[level], want)
		}
	}
}

// Most severe first, then nearest. A severe warning forty miles away matters
// more than an alert down the road, which is the opposite of how a plain
// distance sort would order them.
func TestSeverityLeadsAndDistanceFollows(t *testing.T) {
	got := renderFloods([]flood{
		{Level: 1, Area: "Severn at Bewdley", Distance: 40},
		{Level: 3, Area: "Avon at Bath", Distance: 2},
	}, true, false)

	severe := strings.Index(got, "Bewdley")
	alert := strings.Index(got, "Bath")
	if severe < 0 || alert < 0 {
		t.Fatalf("both warnings should be listed:\n%s", got)
	}
	if severe > alert {
		t.Errorf("the alert two km away is above the severe warning:\n%s", got)
	}
}

// An empty answer says which country it is empty about. "No flood warnings"
// for a question about Cardiff would be true of this feed and false of Wales.
func TestNothingInForceSaysWhereItLooked(t *testing.T) {
	got := renderFloods(nil, true, false)
	for _, want := range []string{"England only", "SEPA", "Natural Resources Wales"} {
		if !strings.Contains(got, want) {
			t.Errorf("an empty answer does not mention %s:\n%s", want, got)
		}
	}
}

// And a full one says it too, because somebody reading three warnings for
// Bristol has even more reason to think the list is complete.
func TestAFullAnswerSaysItToo(t *testing.T) {
	got := renderFloods([]flood{{Level: 2, Area: "Avon"}}, false, false)
	if !strings.Contains(got, "England only") {
		t.Errorf("a non-empty answer does not say what it covers:\n%s", got)
	}
}
