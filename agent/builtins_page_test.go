package agent

// The instance's own agents, listed once.

import (
	"os"
	"strings"
	"testing"
)

// Once, not twice.
//
// The roster page has looped over PlatformNames and drawn a row per built-in
// for a long time. The comment underneath that loop said the opposite — that
// they had been removed, and why — so the file argued with itself, and a second
// listing was added underneath on the strength of the prose. Micro and Code
// then appeared twice on the page, which is how it was noticed.
//
// This reads the source rather than the rendered page because the registry is
// empty in a unit test, so there is nothing to count. What can be checked is
// that the page draws them from one place.
func TestTheBuiltInsAreListedOnceOnTheRoster(t *testing.T) {
	src, err := os.ReadFile("roster_page.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if n := strings.Count(body, "platformSeenRow(id, owner)"); n != 1 {
		t.Errorf("the roster draws the platform rows %d times, want 1", n)
	}
	if strings.Contains(body, "builtinsSection(true)") {
		t.Error("the roster draws a second listing of the instance's agents under\n" +
			"the one it already draws at the top — that is Micro and Code twice\n" +
			"on one page, which is the bug this test exists for")
	}
	// And the prose agrees with the code. The stale comment is what caused the
	// duplicate: somebody read "not listed here" and added a listing.
	if strings.Contains(body, "The instance's own agents are not listed here") {
		t.Error("the comment says the built-ins are not listed and the loop above\n" +
			"it lists them — the file arguing with itself is what produced the\n" +
			"duplicate in the first place")
	}
}

// Signed out, the page is those agents, and that listing is the other one.
//
// It is a different page for a different reader: a stranger has no roster, so
// bouncing them to /login sends them to something empty by definition.
func TestAStrangerGetsTheBuiltInsRatherThanALoginForm(t *testing.T) {
	src, err := os.ReadFile("roster_page.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "builtinsPage()") {
		t.Error("signed out, /agents does not show what this instance can already\n" +
			"do — so the public page is a login form for an empty roster")
	}
}
