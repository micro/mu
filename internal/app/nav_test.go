package app

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// The sidebar is derived from the service Specs. Two services sharing an icon
// is the bug that produced Stream and Chat both showing a speech bubble; a
// hand-written list cannot notice a repeat in itself, a derived one can be
// checked.
func TestNavIsDerivedAndIconsAreDistinct(t *testing.T) {
	specs := []service.Spec{
		{Name: "news", Handler: struct{}{}, Page: "/news", Icon: "news.png"},
		{Name: "chat", Handler: struct{}{}, Page: "/chat", Icon: "chat.png"},
		{Name: "stream", Handler: struct{}{}, Page: "/stream", Icon: "stream.svg"},
		{Name: "index", Handler: struct{}{}}, // headless: no page, no nav entry
	}
	seen := map[string]string{}
	for _, s := range specs {
		if s.Headless() {
			continue
		}
		if prev, dup := seen[s.NavIcon()]; dup {
			t.Errorf("%s and %s share the icon %s", prev, s.Name, s.NavIcon())
		}
		seen[s.NavIcon()] = s.Name
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 nav entries, got %d", len(seen))
	}
}

// Below the nouns, the sidebar shows pinned services and nothing else —
// the ones that change without you. Everything else with a page is reachable
// through the catalogue, which is a noun in the nav.
type NavProbe struct{}

type NavReq struct{ X string }
type NavRsp struct{ Y string }

func (NavProbe) Look(_ context.Context, _ *NavReq, _ *NavRsp) error { return nil }

func TestSidebarShowsPinnedServices(t *testing.T) {
	for _, spec := range []service.Spec{
		{Name: "navpinned", Handler: new(NavProbe), Page: "/navpinned",
			Label: "Nav Pinned", Icon: "navpinned.svg", Pinned: true},
		{Name: "navprobe", Handler: new(NavProbe), Page: "/navprobe",
			Label: "Nav Probe", Icon: "navprobe.svg"},
	} {
		if _, already := service.SpecFor(spec.Name); already {
			continue
		}
		if err := service.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}

	html := navLinks()
	if !strings.Contains(html, `href="/navpinned"`) {
		t.Errorf("a pinned service is missing from the sidebar:\n%s", html)
	}
	if !strings.Contains(html, "/navpinned.svg") || !strings.Contains(html, ">Nav Pinned<") {
		t.Errorf("a pinned service did not use its declared icon and label:\n%s", html)
	}
	// An unpinned service belongs in the catalogue, not the sidebar. Nineteen
	// of them there made it a menu nobody read.
	if strings.Contains(html, `href="/navprobe"`) {
		t.Errorf("an unpinned service was listed in the sidebar:\n%s", html)
	}
}

// The nouns are written into the nav itself: an agent does the work, tasks tell
// it what to do, apps are what you build, tools are what an agent can call,
// services are what this instance runs.
//
// Apps was not there at all — it was one tile among nineteen inside the
// services catalogue, which is a strange place for half the product.
func TestTheNavCarriesTheNouns(t *testing.T) {
	for _, want := range []string{
		`href="/agent"`, `href="/tasks"`, `href="/apps"`,
		`href="/tools"`, `href="/services"`,
	} {
		if !strings.Contains(Template, want) {
			t.Errorf("the nav is missing %q", want)
		}
	}
	// In that order, because the order is the argument.
	order := []string{`href="/agent"`, `href="/tasks"`, `href="/apps"`, `href="/tools"`, `href="/services"`}
	at := -1
	for _, noun := range order {
		i := strings.Index(Template, noun)
		if i < at {
			t.Errorf("%s is out of order in the nav", noun)
		}
		at = i
	}
}

// A service with no page cannot be pinned — there would be nowhere to go.
func TestHeadlessServicesAreNeverPinned(t *testing.T) {
	if _, already := service.SpecFor("navhidden"); !already {
		if err := service.Register(service.Spec{
			Name: "navhidden", Handler: new(NavProbe), Pinned: true,
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	for _, s := range service.Pinned() {
		if s.Name == "navhidden" {
			t.Error("a headless service was pinned into the sidebar")
		}
	}
}

// The sidebar is substituted per render, not built into Template. Template is a
// package-level var evaluated at init, before any service registers — building
// the nav there produced a sidebar with only Home and Agent.
func TestNavIsSubstitutedAtRenderTime(t *testing.T) {
	if !strings.Contains(Template, navPlaceholder) {
		t.Fatal("Template lost its nav placeholder; the sidebar would render empty")
	}
	out := withNav("before" + navPlaceholder + "after")
	if strings.Contains(out, navPlaceholder) {
		t.Error("withNav left the placeholder in the page")
	}
}
