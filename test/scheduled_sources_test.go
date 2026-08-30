package test

// The two scheduled writers run with nobody watching, as Micro's own account.
//
// That is fine for a headline and a share price and would not be fine for
// anything else, so the fixed list it reads has to stay inside three
// properties: free, so an unattended loop cannot spend; unscoped, so it cannot
// read one person's mail or notes; and not destructive, for the reason every
// other door refuses those.
//
// Checked against the specs rather than against a second copy of the list,
// because a second copy is a thing to forget. Widening either list is meant to
// be one line, and this is what makes that safe: add a tool that charges or is
// account-scoped and the build says so.
//
// Both agents, one test. agent/digest publishes a blog post every morning and
// agent/brief writes the line at the top of Home every hour; they read
// different lists for different reasons and the reason they are safe is the
// same one.

import (
	"strings"
	"testing"

	"mu/agent/brief"
	"mu/agent/digest"
	"mu/internal/service"
)

func TestTheScheduledWritersReadOnlyFreeUnscopedTools(t *testing.T) {
	registerAll(t)

	if n := len(digest.Sources()); n < 5 {
		t.Fatalf("the digest reads %d tools — either the list shrank to news and "+
			"markets again or this scan is broken", n)
	}
	if n := len(brief.Sources()); n == 0 {
		t.Fatal("the brief reads nothing, so it writes about nothing")
	}

	names := append(digest.Sources(), brief.Sources()...)
	for _, name := range names {
		svc, method, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("%q is not a service_method tool name", name)
			continue
		}

		spec, found := service.SpecFor(svc)
		if !found {
			t.Errorf("%s names no service — the writer would log a failure every "+
				"run and go out without it", name)
			continue
		}
		if spec.Scoped {
			t.Errorf("%s is account-scoped, and it is called as Micro — what everybody "+
				"reads would be written from whatever Micro's own %s holds", name, svc)
		}

		var ep service.Endpoint
		for n, e := range spec.Endpoints {
			if strings.EqualFold(n, method) {
				ep, found = e, true
				break
			}
		}
		if !found {
			t.Errorf("%s names no method on %s", name, svc)
			continue
		}
		if ep.Cost != "" {
			t.Errorf("%s costs %q, and it is read by a scheduler with nobody watching",
				name, ep.Cost)
		}
		if ep.Destructive {
			t.Errorf("%s is destructive and is being called by a scheduler", name)
		}
	}
}
