package setup

// The model is optional, and a fresh instance is usable without one.
//
// Every branch of ApplyProvider refused, so an instance could not be finished
// without an account at a model vendor. That made a utility something you
// cannot start using until you have signed up for somebody else's product,
// which is the opposite of what running your own server is for — and it is not
// even true of most of what is here: mail, IMAP, SMTP, XMPP, files, notes and
// the record all work with no model at all.

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

// Setup finishes with no provider, and writes no provider settings.
func TestAnInstanceCanStartWithNoModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Load()

	for _, later := range []string{"", "later", "none"} {
		if err := ApplyProvider(later, "", ""); err != nil {
			t.Errorf("ApplyProvider(%q) refused: %v", later, err)
		}
	}
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY"} {
		if v := settings.Get(k); v != "" {
			t.Errorf("choosing no provider wrote %s=%q", k, v)
		}
	}
}

// A provider that was chosen is still checked. "Optional" must not become
// "silently accepts a blank key", which would trade a clear refusal at setup
// for an agent that fails later with no reason attached.
func TestChoosingAProviderStillNeedsItsKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings.Load()

	for _, p := range []string{"claude", "atlas", "openrouter"} {
		if err := ApplyProvider(p, "", ""); err == nil {
			t.Errorf("%s was accepted with no key", p)
		}
	}
	if err := ApplyProvider("nonsense", "k", ""); err == nil {
		t.Error("an unknown provider was accepted")
	} else if !strings.Contains(err.Error(), "later") {
		t.Errorf("the refusal does not mention leaving it for later: %v", err)
	}
}

// And the form offers it, defaulted, rather than hiding it in an empty value.
// A person choosing "not yet" should be able to see that they chose it.
func TestTheFormOffersStartingWithoutOne(t *testing.T) {
	page := render("")
	if !strings.Contains(page, `value="later" checked`) {
		t.Error("starting without a model is not offered as the default choice")
	}
	if !strings.Contains(page, "optional") {
		t.Error("nothing on the form says the provider is optional")
	}
	// And it says what still works, so "not yet" does not read as "not useful".
	if !strings.Contains(page, "work without one") {
		t.Error("the form does not say what an instance with no model can still do")
	}
}
