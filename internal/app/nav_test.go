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

// The rendered markup must link each registered service at its declared route.
type NavProbe struct{}

type NavReq struct{ X string }
type NavRsp struct{ Y string }

func (NavProbe) Look(_ context.Context, _ *NavReq, _ *NavRsp) error { return nil }

func TestNavLinksRenderDeclaredRoutes(t *testing.T) {
	if err := service.Register(service.Spec{
		Name: "navprobe", Handler: new(NavProbe), Page: "/navprobe",
		Label: "Nav Probe", Icon: "navprobe.svg",
	}); err != nil {
		t.Fatal(err)
	}

	html := navLinks()
	if !strings.Contains(html, `href="/navprobe"`) {
		t.Fatalf("registered service missing from the sidebar:\n%s", html)
	}
	if !strings.Contains(html, "/navprobe.svg") {
		t.Errorf("declared icon not used:\n%s", html)
	}
	if !strings.Contains(html, ">Nav Probe<") {
		t.Errorf("declared label not used:\n%s", html)
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
