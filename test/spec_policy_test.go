package test

import (
	"testing"

	"mu/tool"

	"mu/internal/api"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/blog"
	"mu/service/notes"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/images"
	"mu/service/index"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	user "mu/service/user"
	"mu/service/video"
	"mu/service/weather"
	"mu/service/web"
)

// allSpecs is every service main() registers. Keep it complete: a Spec missing
// here is a service the policy and documentation tests never see.
//
// memory was missing for exactly that long. It is headless, so nothing on a
// page pointed at the gap, and the documentation tests concluded there was
// nothing to document — three tools absent from the README and from the
// architecture table, with no test able to say so. A service left out of this
// list is invisible to the very checks that exist to notice that.
func allSpecs() []service.Spec {
	return []service.Spec{
		apps.Spec, blog.Spec, chat.Spec, contacts.Spec, docs.Spec, events.Spec,
		files.Spec, images.Spec, index.Spec, mail.Spec, markets.Spec,
		notes.Spec, news.Spec, places.Spec, prayer.Spec, sms.Spec, social.Spec,
		stream.Spec, tasks.Spec, user.Spec, video.Spec, weather.Spec, web.Spec,
	}
}

// registerAll stands up every service from its own Spec — the same
// declarations main() registers.
func registerAll(t *testing.T) {
	t.Helper()
	for _, s := range allSpecs() {
		// Registering starts a server, and two tests in one binary both calling
		// this raced for the port — "already listening on 127.0.0.1:11361",
		// which reads like a bug in the thing under test and is not one.
		if _, already := service.SpecFor(s.Name); already {
			continue
		}
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
}

// The real specs must reproduce the policy the deleted hand-written maps held.
func TestSpecsReproduceTheOldPolicy(t *testing.T) {
	for _, s := range []service.Spec{mail.Spec, index.Spec, tasks.Spec, web.Spec, blog.Spec, user.Spec} {
		// Idempotent for the same reason registerAll is: another test in this
		// binary may have registered these already, and registering twice
		// races for the port.
		if _, already := service.SpecFor(s.Name); already {
			continue
		}
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
	// accountScoped, deleted from internal/service/dynamic.go
	for _, n := range []string{"mail", "tasks", "user"} {
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
	for _, tool := range []string{"mail_inbox", "tasks_list", "user_saved"} {
		if service.GuestAllowedTool(tool) {
			t.Errorf("%s must stay closed to guests", tool)
		}
	}
	// destructiveTools, deleted from agent/native.go
	if !service.Destructive("blog", "Delete") || !service.Destructive("tasks", "Delete") {
		t.Error("a destructive method lost its guard")
	}
	if service.Destructive("blog", "Read") || service.Destructive("tasks", "List") {
		t.Error("a read was marked destructive")
	}
	// agentToolLabels, deleted from agent/native.go
	if got := service.Label("web"); got != "Search" {
		t.Errorf("web label = %q, want Search", got)
	}
	if got := service.Label("mail"); got != "Mail" {
		t.Errorf("mail label = %q, want Mail", got)
	}
}

// The catalogue at /services is derived from the Specs, so the set it renders
// has to be checked against every registered service — not against the eighteen
// anchors that used to be written out by hand.
//
// service.Nav() backs the catalogue now rather than the sidebar; the sidebar
// shows service.Pinned(). The checks are the same either way: no two services
// may share an icon, a route or a label.
func TestNavCoversEveryPagedServiceExactlyOnce(t *testing.T) {
	registerAll(t)

	nav := service.Nav()
	icons, routes, labels := map[string]string{}, map[string]string{}, map[string]string{}
	for _, s := range nav {
		if prev, dup := icons[s.NavIcon()]; dup {
			t.Errorf("%s and %s share the icon %q", prev, s.Name, s.NavIcon())
		}
		icons[s.NavIcon()] = s.Name
		// Headless services all have the empty route, which is not a clash.
		if s.Page != "" {
			if prev, dup := routes[s.Page]; dup {
				t.Errorf("%s and %s share the route %q", prev, s.Name, s.Page)
			}
			routes[s.Page] = s.Name
		}
		if prev, dup := labels[s.NavLabel()]; dup {
			t.Errorf("%s and %s share the label %q", prev, s.Name, s.NavLabel())
		}
		labels[s.NavLabel()] = s.Name
	}

	// Headless services must not appear.
	for _, name := range []string{"index"} {
		if _, ok := routes["/"+name]; ok {
			t.Errorf("%s is headless and must not be in the sidebar", name)
		}
	}

	// Counted from the Specs rather than written down: a hard-coded total goes
	// stale the moment a service is added, and the number it was checking was
	// the size of registerAll, not the size of the product.
	// Every service, headless or not: the catalogue is what this instance runs,
	// and having a page is a fact about a service's UI rather than about
	// whether it exists.
	if len(nav) != len(allSpecs()) {
		t.Errorf("the catalogue has %d entries, want %d — %v", len(nav), len(allSpecs()), routes)
	}
}

// Every endpoint a service declares is reachable over MCP.
//
// Tools were registered by hand in main() while the agent's tools derived from
// the Specs, so adding an endpoint gave the agent something to call and an MCP
// client nothing. Six had drifted out of reach that way — mail_search,
// places_geocode, chat_rooms, chat_messages, wallet_check, wallet_charge — and
// not one of them was withheld on purpose.
//
// api.DeriveTools closes the gap at startup. This checks it stays closed, so a
// new endpoint cannot go missing between the Spec and the client again.
func TestEveryEndpointIsReachableOverMCP(t *testing.T) {
	registerAll(t)
	tool.DeriveTools()

	for _, s := range allSpecs() {
		for method := range s.Endpoints {
			name := s.Tool(method)
			if !api.HasTool(name) {
				t.Errorf("%s.%s is declared but no client can call %s", s.Name, method, name)
			}
		}
	}
}

// A funded wallet is not accountable for this sending domain.
//
// mail_send was a hand-written registration carrying AccountOnly; it derives
// from the mail Spec now, so the flag has to be on the Endpoint or an anonymous
// payer can send mail from this instance's domain and there is nobody to hold
// to it. Checked against the real Spec rather than a probe, because a probe can
// only prove the mechanism works.
func TestSendingMailNeedsAnAccountNotAWallet(t *testing.T) {
	ep, ok := mail.Spec.Endpoints["Send"]
	if !ok {
		t.Fatal("the mail service no longer declares Send")
	}
	if !ep.AccountOnly {
		t.Error("mail.Send is not account-only — a settled payment would be " +
			"identity enough to send from this domain")
	}
	if ep.Cost == "" {
		t.Error("mail.Send charges nothing, so external delivery is free")
	}
}
