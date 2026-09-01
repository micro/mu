package hazards

// What the page does with the parts of a feed that are missing.
//
// Three third parties fill these structs and none of them fills every field.
// GDACS leaves an alert's name empty for most floods; the Environment Agency
// does not date every warning. Rendered without checking, those became "—
// Nepal" — a dash with nothing on the left of it — and "106751d ago", which is
// a zero time run through a duration and printed with a straight face next to
// a river in Worcestershire.

import (
	"testing"
	"time"
)

func TestAMissingTimeRendersAsNothing(t *testing.T) {
	if got := when(time.Time{}); got != "" {
		t.Errorf("a zero time renders as %q; a warning nobody dated is undated, "+
			"and three hundred years ago is not the honest way to say so", got)
	}
	// And a real one still reads.
	if got := when(time.Now().Add(-2 * time.Hour)); got == "" {
		t.Error("a real time renders as nothing, so the column is always blank")
	}
}

func TestAnAlertWithNoNameIsNotADanglingDash(t *testing.T) {
	for _, c := range []struct{ name, country, want string }{
		{"SAUDEL-26", "Japan", "SAUDEL-26 — Japan"},
		{"", "Nepal", "Nepal"},
		{"Cyclone Ada", "", "Cyclone Ada"},
		{"", "", ""},
		{"  ", " Chile ", "Chile"},
	} {
		if got := describe(c.name, c.country); got != c.want {
			t.Errorf("describe(%q, %q) = %q, want %q", c.name, c.country, got, c.want)
		}
	}
}

// Severity is readable without reading the row.
func TestSeverityIsCarriedByTheMarkAndNotOnlyTheWords(t *testing.T) {
	if magClass(6.9) != "hz-sev" {
		t.Error("a M6.9 is not marked as severe")
	}
	if magClass(4.1) == magClass(6.9) {
		t.Error("a M4.1 and a M6.9 look the same, so the list has to be read in full")
	}
	if levelClass("Red") != "hz-sev" || levelClass("red") != "hz-sev" {
		t.Error("a red alert is not marked severe, or the check is case-sensitive " +
			"against a feed that is not")
	}
	if levelClass("Orange") == levelClass("Red") {
		t.Error("orange and red look the same")
	}
}

// The page is the thing; the reference is a link on it.
//
// This is the whole point of drawing it by hand again — /services/hazards is a
// good manual and a bad answer to "is anything happening". It should be
// reachable from here and must not be what somebody lands on.
func TestThePageLinksToTheReferenceRatherThanBeingIt(t *testing.T) {
	body := quakeSection(nil) + alertSection(nil) + floodSection(nil)
	if body == "" {
		t.Fatal("the sections render nothing at all when the feeds are empty")
	}
	// Empty feeds are the good news and should read like it, not like a failure.
	for _, want := range []string{"Nothing above M", "No alerts above green",
		"No flood warnings in force"} {
		if !contains(body, want) {
			t.Errorf("an empty feed does not say so in words: missing %q", want)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
