package agent

import "testing"

// An agent is a place, with an address you can say out loud.
//
// It was /agent?id=2d5e1f4d-edd1-488c-9b49-5fb2bf7e518f — a page you cannot
// bookmark meaningfully, cannot tell from another at a glance, and cannot type.
// /mail and /news are places; an agent should be one too.
func TestAnAgentIsNamedInThePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// The mail tag wins, because an agent answering at you+research@ should be
	// at /agent/research: one name for the same thing is why tags exist.
	if got := Slug(&Agent{Name: "Morning Briefer", Tag: "research"}); got != "research" {
		t.Errorf("Slug with a tag = %q, want %q", got, "research")
	}
	// Otherwise the display name, in the same shape a tag would take.
	if got := Slug(&Agent{Name: "Morning Briefer"}); got != "morningbriefer" {
		t.Errorf("Slug = %q, want %q", got, "morningbriefer")
	}
	// The built-in has no roster entry, so it needs a name of its own to be
	// addressable at all.
	if got := Slug(nil); got != DefaultSlug {
		t.Errorf("the default agent is called %q, want %q", got, DefaultSlug)
	}
}

// A name that matches nothing is not quietly the default. Serving a different
// agent for a typo is worse than saying the page is not there.
func TestAnUnknownNameIsNotTheDefaultAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, ok := BySlug("nobody", DefaultSlug); !ok {
		t.Error("the default agent is not reachable by name")
	}
	if _, ok := BySlug("nobody", ""); !ok {
		t.Error("an empty name should mean the default agent")
	}
	if id, ok := BySlug("nobody", "someone-elses-agent"); ok {
		t.Errorf("an unknown name resolved to %q instead of not resolving", id)
	}
}

// A guest has no roster, so the only agent they can be on a page about is the
// default one.
func TestAGuestOnlyHasTheDefaultAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, ok := agentSlugTarget("", DefaultSlug, true); !ok {
		t.Error("a signed-out visitor cannot reach the default agent")
	}
	if _, ok := agentSlugTarget("", "someones-private-agent", true); ok {
		t.Error("a signed-out visitor resolved somebody's agent by name")
	}
}

// Path is what every link should be built from, so the roster and the redirect
// cannot disagree about where an agent lives.
func TestThePathToAnAgentIsTheOneThingLinksUse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if got, want := Path("nobody", ""), "/agent/"+DefaultSlug; got != want {
		t.Errorf("Path to the default = %q, want %q", got, want)
	}
	// An id nobody owns falls back to the default rather than building a link
	// into a 404 — a stale link should land somewhere usable.
	if got, want := Path("nobody", "gone"), "/agent/"+DefaultSlug; got != want {
		t.Errorf("Path for a removed agent = %q, want %q", got, want)
	}
}
