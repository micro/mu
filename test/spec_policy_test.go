package test

import (
	"os"
	"strings"
	"testing"

	"mu/tool"

	"mu/internal/api"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/archive"
	"mu/service/blog"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/email"
	"mu/service/events"
	"mu/service/files"
	"mu/service/flights"
	"mu/service/food"
	"mu/service/hazards"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/notes"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/recall"
	"mu/service/routes"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/text"
	"mu/service/tiles"
	"mu/service/transit"
	"mu/service/video"
	"mu/service/wallet"
	"mu/service/weather"
	"mu/service/web"
	whatsappsvc "mu/service/whatsapp"
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
		apps.Spec, archive.Spec, blog.Spec, chat.Spec, contacts.Spec, docs.Spec, events.Spec,
		email.Spec, files.Spec, flights.Spec, food.Spec, hazards.Spec, images.Spec, mail.Spec, markets.Spec,
		notes.Spec, news.Spec, places.Spec, prayer.Spec, recall.Spec, routes.Spec,
		sms.Spec,
		social.Spec,
		stream.Spec, tasks.Spec, text.Spec, tiles.Spec, transit.Spec, video.Spec,
		wallet.Spec, weather.Spec, web.Spec,
		whatsappsvc.Spec,
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
	for _, s := range []service.Spec{mail.Spec, tasks.Spec, web.Spec, blog.Spec, notes.Spec} {
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
	// accountScoped, deleted from internal/service/dynamic.go. "user" was on
	// this list and is not a service any more — what an account saves, hides
	// and blocks is furniture, not a capability. See internal/user.
	for _, n := range []string{"mail", "tasks", "notes"} {
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
	// accountScoped map marked it scoped, which closed it in the agent — while
	// the micro-agent's own allowlist let it through. Two
	for _, tool := range []string{"mail_inbox", "tasks_list", "notes_list"} {
		if service.PublicTool(tool) {
			t.Errorf("%s reaches one person's data and must not run in a group channel", tool)
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
// mail_send was a hand-written registration carrying the flag; it derives from
// the mail Spec now, so Needs has to say Account on the Endpoint or an anonymous
// payer can send mail from this instance's domain and there is nobody to hold
// to it. Checked against the real Spec rather than a probe, because a probe can
// only prove the mechanism works.
func TestSendingMailNeedsAnAccountNotAWallet(t *testing.T) {
	ep, ok := mail.Spec.Endpoints["Send"]
	if !ok {
		t.Fatal("the mail service no longer declares Send")
	}
	if ep.Needs != service.Account {
		t.Error("mail.Send does not require an account — a settled payment would be " +
			"identity enough to send from this domain")
	}
	// Send routes by recipient and has two prices, so it declares no flat Cost
	// and charges itself — the shape sms_send has for the same reason.
	//
	// This used to require the opposite: a Cost on Send, and a second endpoint
	// called Email, on the argument that a price depending on an argument cannot
	// be shown in the catalogue. That was true and it cost more than it bought.
	// A caller had to know whether a recipient held an account here before
	// choosing which tool to call, which is a fact about our database rather
	// than about writing to somebody.
	//
	// What has to survive is not the split. It is that mail leaving this
	// instance is charged and is answerable to an account, and both are checked
	// below.
	if ep.Cost != "" {
		t.Errorf("mail.Send declares a flat cost of %s, but it has two prices — "+
			"the gateway would charge local delivery at the external rate or the "+
			"other way round", ep.Cost)
	}

	// Charged in the package, since the gateway cannot. Read from the source
	// because there is no other way to state it: an endpoint with no Cost that
	// forgot to charge looks exactly like one that meant to be free.
	//
	// Two files, and which is which is the point. Local delivery is charged
	// where the endpoint routes; mail that leaves is charged in outbound.go,
	// which is the single path everything that leaves goes through — the tool,
	// the JSON API and the compose form all reach it, so the price and the gate
	// are applied once rather than copied three times.
	for file, op := range map[string]string{
		"../service/mail/service.go":  "quota.OpMailSend",
		"../service/mail/outbound.go": "quota.OpExternalEmail",
	} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), op) {
			t.Errorf("%s never charges %s, so that route is free", file, op)
		}
	}

	// mail_email named a real distinction and agents learned it. It resolves to
	// the same call now, but it has to keep resolving.
	found := false
	for _, a := range ep.Aliases {
		if a == "mail_email" {
			found = true
		}
	}
	if !found {
		t.Error("mail_email is gone rather than aliased — an agent that learned " +
			"it now gets a tool-not-found for something it used yesterday")
	}
}
