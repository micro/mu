package service

import (
	"path/filepath"
	"strings"
	"testing"
)

// A tool's name is derived, not written: service + "_" + method. That only
// reads well if the two halves say different things, which holds when a
// service is named for a domain (news, mail, places) and a method for what it
// does with that domain.
//
// A service named for an action has nowhere left to go: its main method has to
// repeat it. That is how "search" ended up with search.Search, deriving the
// tool name search_search. The capability moved to web.Search, and this test
// stops the next one arriving.
func TestNoMethodRepeatsItsService(t *testing.T) {
	forEachService(t, func(svc string, methods []string) {
		for _, m := range methods {
			if strings.EqualFold(m, svc) {
				t.Errorf("%s.%s derives the tool name %s_%s — name the service for a "+
					"domain and the method for what it does",
					svc, m, svc, strings.ToLower(m))
			}
		}
	})
}

// Two endpoints deriving the same tool name would make one of them
// unreachable, silently.
func TestDerivedToolNamesAreUnique(t *testing.T) {
	seen := map[string]string{}
	forEachService(t, func(svc string, methods []string) {
		for _, m := range methods {
			name := svc + "_" + strings.ToLower(m)
			if prev, dup := seen[name]; dup {
				t.Errorf("%s.%s and %s both derive %q", svc, m, prev, name)
				continue
			}
			seen[name] = svc + "." + m
		}
	})
}

// forEachService walks the service packages, handing each one its name and the
// RPC methods it declares. The directory name is the service name — that is
// the convention, and SERVICE_REGISTRY.md documents it.
func forEachService(t *testing.T, fn func(service string, methods []string)) {
	t.Helper()
	root := repoRoot(t)
	dirs, err := filepath.Glob(filepath.Join(root, "service", "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	checked := 0
	for _, dir := range dirs {
		methods, _, ok := scanService(t, dir)
		if !ok {
			continue // a package with no handler, e.g. search
		}
		checked++
		fn(filepath.Base(dir), methods)
	}
	if checked < 15 {
		t.Fatalf("only scanned %d services; the scan is not finding them", checked)
	}
}
