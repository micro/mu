package test

// The daily briefing runs with nobody watching, as Micro's own account.
//
// That is fine for a headline and a share price and would not be fine for
// anything else, so the fixed list it reads has to stay inside three
// properties: free, so an unattended loop cannot spend; unscoped, so it cannot
// read one person's mail or notes; and not destructive, for the reason every
// other door refuses those.
//
// Checked against the specs rather than against a second copy of the list,
// because a second copy is a thing to forget. Widening digestSources is meant
// to be one line, and this is what makes that safe: add a tool that charges or
// is account-scoped and the build says so.

import (
	"strings"
	"testing"

	"mu/agent/digest"
	"mu/internal/service"
)

func TestTheBriefingReadsOnlyFreeUnscopedTools(t *testing.T) {
	registerAll(t)

	sources := digest.Sources()
	if len(sources) < 5 {
		t.Fatalf("the briefing reads %d tools — either the list shrank to news and "+
			"markets again or this scan is broken", len(sources))
	}

	for _, name := range sources {
		svc, method, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("%q is not a service_method tool name", name)
			continue
		}

		spec, found := service.SpecFor(svc)
		if !found {
			t.Errorf("%s reads %s, and there is no %s service — the briefing would log "+
				"a failure every day and publish without it", name, name, svc)
			continue
		}
		if spec.Scoped {
			t.Errorf("%s is account-scoped, and the briefing calls it as Micro — a "+
				"public post would be written from whatever Micro's own %s holds",
				name, svc)
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
			t.Errorf("%s costs %q, and the briefing runs daily with nobody watching",
				name, ep.Cost)
		}
		if ep.Destructive {
			t.Errorf("%s is destructive and is being called by a scheduler", name)
		}
	}
}
