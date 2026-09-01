package test

// Every service answers at its own name.
//
// A service is a noun in this product — news, weather, markets, hazards — and
// the address bar is the first place somebody tries it. Thirty-four of the
// thirty-five answered at /<name>. hazards was a 404, and had been since its
// hand-drawn page was deleted: deleting the page was right, the derived page at
// /services/hazards is strictly more than the form it replaced, but the route
// went with the page and nothing put the name back.
//
// That is the failure this pins. Not "every service needs a bespoke page" —
// most should not have one — but that the name has to reach something. A
// redirect to the derived page is a perfectly good answer; a 404 is not.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A service directory under service/ is a service.
func serviceNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(at("service"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	if len(out) < 20 {
		t.Fatalf("found only %d services — this test is reading the wrong directory", len(out))
	}
	return out
}

func TestEveryServiceAnswersAtItsOwnName(t *testing.T) {
	b, err := os.ReadFile(at("internal/server/routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	routes := string(b)

	// Registered top-level paths, however the handler is named.
	handled := regexp.MustCompile(`http\.(HandleFunc|Handle)\("(/[a-z0-9]+)`)
	have := map[string]bool{}
	for _, m := range handled.FindAllStringSubmatch(routes, -1) {
		have[m[2]] = true
	}

	// Services whose name is not their route, with the reason. Each of these
	// is reachable — the point of the list is that the exception was decided
	// rather than forgotten.
	except := map[string]string{
		// The four surfaces of the product own their own words, and the
		// service behind each is reached through them.
		"mail":    "/inbox is the surface; /mail is registered separately for the client",
		"chat":    "/chat is the surface",
		"stream":  "/stream is the surface",
		"users":   "a person is /@name, not /users",
		"archive": "/archive is the surface",
		"recall":  "/recall is the surface",
		"notes":   "/notes is the surface",
		"tasks":   "/tasks is the surface",
		"apps":    "/apps is the surface",
	}

	var missing []string
	for _, name := range serviceNames(t) {
		if have["/"+name] {
			continue
		}
		if _, ok := except[name]; ok {
			continue
		}
		missing = append(missing, name)
	}

	if len(missing) > 0 {
		t.Errorf("%d service(s) do not answer at their own name:\n  %s\n"+
			"A service is a noun and the address bar is the first place somebody\n"+
			"tries it. It does not need a page of its own — /services/<name> is\n"+
			"derived for every service and a redirect to it is a fine answer — but\n"+
			"the name has to reach something rather than 404.",
			len(missing), strings.Join(missing, "\n  "))
	}

	// And nothing stale in the exception list: an entry for a service that
	// does answer is a note about a decision that has been reversed.
	for name := range except {
		if have["/"+name] {
			continue
		}
		found := false
		for _, s := range serviceNames(t) {
			if s == name {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is listed as an exception and is not a service any more", name)
		}
	}
}
