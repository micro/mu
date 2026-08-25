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
	"mu/service/browser"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/flights"
	"mu/service/food"
	"mu/service/hazards"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/maps"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/notes"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/recall"
	"mu/service/routes"
	"mu/service/shell"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/text"
	"mu/service/transit"
	"mu/service/users"
	"mu/service/video"
	"mu/service/wallet"
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
		apps.Spec, archive.Spec, blog.Spec, chat.Spec, contacts.Spec, docs.Spec, events.Spec,
		files.Spec, flights.Spec, food.Spec, hazards.Spec, images.Spec, mail.Spec, markets.Spec,
		notes.Spec, news.Spec, places.Spec, prayer.Spec, recall.Spec, routes.Spec,
		sms.Spec,
		social.Spec,
		stream.Spec, tasks.Spec, text.Spec, maps.Spec, transit.Spec, video.Spec,
		users.Spec, wallet.Spec, weather.Spec, web.Spec, browser.Spec, shell.Spec,
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
	// And the other half of the rule, which lived in agent/micro and went with
	// the executor there. A gate that says no to everything passes every test
	// phrased as "this must not be public", so the ones that must be are named
	// here beside them.
	for _, tool := range []string{"weather_forecast", "news_list", "markets_list", "web_search"} {
		if !service.PublicTool(tool) {
			t.Errorf("%s has nothing private in it and should run in a group channel", tool)
		}
	}
	// destructiveTools, deleted from agent/native.go
	if !service.Destructive("blog", "Delete") || !service.Destructive("tasks", "Delete") {
		t.Error("a destructive method lost its guard")
	}
	if service.Destructive("blog", "Read") || service.Destructive("tasks", "List") {
		t.Error("a read was marked destructive")
	}
	// agentToolLabels, deleted from agent/native.go.
	//
	// web was labelled "Search" here, which is what it was called before it was
	// renamed for its domain rather than its main action. Reproducing the old
	// policy is the job of this test and that part of the old policy was the
	// bug — see TestAServiceLabelIsItsOwnName.
	if got := service.Label("web"); got != "Web" {
		t.Errorf("web label = %q, want Web", got)
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
	// One file, and that is the point. Both halves of sending — a message that
	// leaves the instance and one that stays on it — are charged in
	// outbound.go, at ReplyOut and at DeliverHere. There were two prices and
	// two enforcement stories, and the local one was free on every door: the
	// tool charged, the compose form charged after it had already sent, and
	// submission did not charge at all.
	src, err := os.ReadFile("../service/mail/outbound.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "quota.OpMailSend") {
		t.Error("outbound.go never charges quota.OpMailSend, so sending is free")
	}

	// And nothing else charges it, because nothing else is allowed to decide.
	for _, other := range []string{"../service/mail/service.go", "../service/mail/mail.go",
		"../service/mail/submission.go", "../service/mail/client.go"} {
		b, err := os.ReadFile(other)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "quota.OpMailSend") {
			t.Errorf("%s charges for sending on terms of its own — that is DeliverHere's "+
				"job, and a second charge is a second set of rules to keep in step", other)
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

// A method named for a mutation declares that it mutates.
//
// Writes is a flag somebody has to remember, which is exactly how the thing it
// replaced went wrong: Destructive was asked to mean both "withhold from the
// model" and "refuse a GET", and every method that wrote but was safe for an
// agent to hold fell through the gap. Replacing one forgettable flag with
// another would be no fix at all.
//
// So the rule is checkable, and it is checkable because the naming convention
// in AGENTS.md is real: "An action is a verb, and says what it changes." A
// method named Add, Create, Send or Pay changes something by construction. The
// list below is the mutating half of that convention.
//
// It cannot catch a writer named for a question — a Fetch that stores what it
// fetched — and that is not a reason to skip the ones it does catch. A rule you
// can only follow by remembering is the one that was just removed.
func TestAMethodNamedForAMutationSaysItWrites(t *testing.T) {
	registerAll(t)

	mutating := []string{
		"add", "build", "cancel", "clear", "create", "delete", "edit", "fork",
		"generate", "import", "move", "pay", "post", "publish", "put", "remove",
		"rename", "revoke", "run", "save", "send", "set", "share", "update",
		"upload", "verify", "write",
	}

	is := map[string]bool{}
	for _, v := range mutating {
		is[v] = true
	}

	var checked int
	for _, sp := range service.Specs() {
		for name, ep := range sp.Endpoints {
			// The first word, not a prefix. "add" is a prefix of Address, and
			// places.Address is a geocoder.
			if !is[firstWord(name)] {
				continue
			}
			checked++
			if !ep.Writes && !ep.Destructive {
				t.Errorf("%s.%s is named for a mutation but declares neither "+
					"Writes nor Destructive, so the HTTP door will serve it on a "+
					"GET — a URL that changes your data when something follows it",
					sp.Name, name)
			}
		}
	}

	if checked < 15 {
		t.Fatalf("only %d endpoints matched a mutating verb — this scan is broken", checked)
	}
}

// firstWord is the leading CamelCase word of a method name, lowered:
// SurfaceBreaking is "surface", Address is "address".
func firstWord(name string) string {
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			return strings.ToLower(name[:i])
		}
	}
	return strings.ToLower(name)
}

// A service's page is at its own name.
//
// "service name == directory == route == nav label == tool prefix, with no
// exceptions in it" is in AGENTS.md and was not checkable, so there was one:
// service/web served its page at /search while everything else under it —
// /web/fetch, /web/read, /web/preview — was already at /web. One service, its
// URL tree split in half, and the only way to know was to notice.
//
// /services/<name> is the other legal answer, for a service whose page is its
// reference because it had nothing its card did not: weather and hazards.
//
// Nothing here says the old address must stop working. /search is a permanent
// redirect, which is what an address people have and search engines hold is
// worth.
func TestAServicePageIsAtItsOwnName(t *testing.T) {
	registerAll(t)

	var checked int
	for _, s := range service.Specs() {
		if s.Page == "" {
			continue // headless: reachable at /services/<name> like everything else
		}
		checked++
		if s.Page != "/"+s.Name && s.Page != "/services/"+s.Name {
			t.Errorf("service %q has its page at %q, want /%s or /services/%s",
				s.Name, s.Page, s.Name, s.Name)
		}
	}
	if checked < 20 {
		t.Fatalf("only %d services have pages — this scan is broken", checked)
	}
}

// And the label a person reads is the name too, unless it is only capitalising
// it. Docs, Routes and SMS set one and all three are their own name typed the
// way it should look; web set "Search", which was the name it had before it was
// renamed for its domain rather than its main action, and it left the sidebar
// disagreeing with the route and the tool prefix.
func TestAServiceLabelIsItsOwnName(t *testing.T) {
	registerAll(t)

	for _, s := range service.Specs() {
		if s.Label == "" {
			continue
		}
		if !strings.EqualFold(strings.ReplaceAll(s.Label, " ", ""), s.Name) {
			t.Errorf("service %q is labelled %q, which is a different word — a "+
				"Label may capitalise the name and nothing else", s.Name, s.Label)
		}
	}
}
