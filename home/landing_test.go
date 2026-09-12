package home

// What the landing says it is, and how somebody joins from it.

import (
	"strings"
	"testing"
)

// The name is followed by what the thing is.
//
// The wordmark stood alone over a box. "Mu" is three characters of Greek and
// says nothing to somebody who arrived from a link; the <title> and the meta
// description both said personal assistant, so the answer existed on the page
// in two places a reader never looks and in none that they do.
func TestTheWordmarkSaysWhatItIs(t *testing.T) {
	body := indexBody()
	brand := strings.Index(body, `class="lbrand"`)
	what := strings.Index(body, `class="lwhat"`)
	if what < 0 || !strings.Contains(body, "A personal assistant") {
		t.Fatal("the landing names itself and never says what it is")
	}
	// Under the name, not over it: the name is still the name.
	if brand < 0 || what < brand {
		t.Error("the caption is above the wordmark it captions")
	}
	// And quiet. A dark line here is a tagline arguing for something, which is
	// the landing page this one was written to stop being.
	if !strings.Contains(body, ".lwhat{color:#888") {
		t.Error("the caption is not in the page's quiet grey; it reads as a pitch")
	}
}

func TestTheLandingOffersOnlyLogin(t *testing.T) {
	got := topRight()
	if got != `<a href="/login">Log in</a>` {
		t.Errorf("unexpected landing navigation: %q", got)
	}
}
