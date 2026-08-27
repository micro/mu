package agent

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

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
	if id, ok := BySlug("nobody", "someone_elses_agent"); ok {
		t.Errorf("an unknown name resolved to %q instead of not resolving", id)
	}
}

// No account, no roster — so a name that is not the default names nothing.
//
// These pages redirect a signed-out visitor to the login now, so this is about
// the empty account id rather than about a guest. It still has to hold:
// resolving somebody's private agent from an empty id would be the same bug
// whichever caller arrived with one.
func TestAnEmptyAccountResolvesOnlyTheDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, ok := agentSlugTarget("", DefaultSlug); !ok {
		t.Error("the default agent does not resolve for an empty account")
	}
	if _, ok := agentSlugTarget("", "someones_private_agent"); ok {
		t.Error("an empty account resolved somebody's agent by name")
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

// The link and the lookup are inverses, and nothing checked that they were.
//
// Path builds /agent/<slug> and BySlug reads it back. Each was tested alone and
// both passed, while Path was being called with an empty owner from the front
// page — so every agent's link came out as /agent/micro, BySlug read "micro"
// as the default, and an agent called Claude opened Micro's chat with Micro's
// conversations beside it. A round trip is the only test that could have caught
// it, because neither half is wrong on its own.
func TestALinkToAnAgentResolvesBackToIt(t *testing.T) {
	const who = "slug_round_trip"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test_secret"}) //nolint:errcheck

	for _, name := range []string{"Claude", "Research", "Night Shift", "Ops 2"} {
		made, _, err := CreateAgent(who, name, "", "", "", nil, false)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		path := Path(who, made.ID)
		if path == "/agent/"+DefaultSlug {
			t.Errorf("%s links to the default agent (%s)", name, path)
		}

		got, ok := BySlug(who, strings.TrimPrefix(path, "/agent/"))
		if !ok {
			t.Errorf("%s: %s resolves to nothing", name, path)
			continue
		}
		if got != made.ID {
			t.Errorf("%s: %s resolves to %q, want %q", name, path, got, made.ID)
		}
		if title := agentTitle(who, got); title != name {
			t.Errorf("%s: the page at %s is titled %q", name, path, title)
		}
	}
}
