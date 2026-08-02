package main

import (
	"testing"

	"mu/internal/service"
	"mu/service/apps"
	"mu/service/blog"
	"mu/service/chat"
	"mu/service/db"
	"mu/service/events"
	"mu/service/images"
	"mu/service/index"
	"mu/service/islam"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/places"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/video"
	"mu/service/wallet"
	"mu/service/weather"
	"mu/service/web"
)

// registerAll stands up every service from its own Spec — the same
// declarations main() registers.
func registerAll(t *testing.T) {
	t.Helper()
	for _, s := range []service.Spec{
		apps.Spec, blog.Spec, chat.Spec, db.Spec, events.Spec, images.Spec,
		index.Spec, islam.Spec, mail.Spec, markets.Spec, news.Spec, places.Spec,
		social.Spec, stream.Spec, video.Spec, wallet.Spec, weather.Spec, web.Spec,
	} {
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
}

// The real specs must reproduce the policy the deleted hand-written maps held.
func TestSpecsReproduceTheOldPolicy(t *testing.T) {
	for _, s := range []service.Spec{mail.Spec, index.Spec, db.Spec, wallet.Spec, web.Spec} {
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
	// accountScoped, deleted from internal/service/dynamic.go
	for _, n := range []string{"mail", "db", "wallet"} {
		if !service.AccountScoped(n) {
			t.Errorf("%s lost its account scoping", n)
		}
	}
	for _, n := range []string{"web", "news", "markets", "stream"} {
		if service.AccountScoped(n) {
			t.Errorf("%s is public and must not be scoped", n)
		}
	}
	// index is deliberately not scoped, and this is a change. The old
	// accountScoped map marked it scoped, which closed it to guests in the
	// agent — while the micro-agent's own allowlist let guests use it. Two
	// lists, two answers. Search adds the caller's mail only when there is a
	// caller, so a guest search returns public indexed content and nothing
	// else; open is the answer that matches what the code does.
	if service.AccountScoped("index") {
		t.Error("index must stay open to guests, who get public content only")
	}
	if !service.GuestAllowedTool("index_search") {
		t.Error("a guest must be able to search public indexed content")
	}
	for _, tool := range []string{"mail_inbox", "db_get", "wallet_balance"} {
		if service.GuestAllowedTool(tool) {
			t.Errorf("%s must stay closed to guests", tool)
		}
	}
	// destructiveTools, deleted from agent/native.go
	if !service.Destructive("wallet", "Charge") || !service.Destructive("db", "Delete") {
		t.Error("a destructive method lost its guard")
	}
	if service.Destructive("wallet", "Balance") || service.Destructive("db", "Get") {
		t.Error("a read was marked destructive")
	}
	// agentToolLabels, deleted from agent/native.go
	if got := service.Label("web"); got != "Search" {
		t.Errorf("web label = %q, want Search", got)
	}
	if got := service.Label("db"); got != "Storage" {
		t.Errorf("db label = %q, want Storage", got)
	}
	if got := service.Label("mail"); got != "Mail" {
		t.Errorf("mail label = %q, want Mail", got)
	}
}

// The sidebar is derived from the Specs, so the set it renders has to be
// checked against every registered service — not against the eighteen anchors
// that used to be written out by hand.
func TestNavCoversEveryPagedServiceExactlyOnce(t *testing.T) {
	registerAll(t)

	nav := service.Nav()
	icons, routes, labels := map[string]string{}, map[string]string{}, map[string]string{}
	for _, s := range nav {
		if prev, dup := icons[s.NavIcon()]; dup {
			t.Errorf("%s and %s share the icon %q", prev, s.Name, s.NavIcon())
		}
		icons[s.NavIcon()] = s.Name
		if prev, dup := routes[s.Page]; dup {
			t.Errorf("%s and %s share the route %q", prev, s.Name, s.Page)
		}
		routes[s.Page] = s.Name
		if prev, dup := labels[s.NavLabel()]; dup {
			t.Errorf("%s and %s share the label %q", prev, s.Name, s.NavLabel())
		}
		labels[s.NavLabel()] = s.Name
	}

	// Headless services must not appear.
	for _, name := range []string{"index", "db"} {
		if _, ok := routes["/"+name]; ok {
			t.Errorf("%s is headless and must not be in the sidebar", name)
		}
	}
	if len(nav) != 16 {
		t.Errorf("sidebar has %d entries, want 16 — %v", len(nav), routes)
	}
}
