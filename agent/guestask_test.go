package agent

// A stranger can ask, and gets nothing private.
//
// The front page's only control is the agent box, and /agent answered every
// signed-out question with 401 "Sign in to ask the agent" — so a visitor's
// first interaction with the product was a sign-up form. ChatGPT, Claude and
// Google all answer without an account; a personal server that refuses to speak
// until you register is asking more than the things it is defined against.
//
// What that costs has to be bounded and it has to be public, and both halves
// are asserted here rather than left to the handler reading correctly.

import (
	"strings"
	"testing"

	"mu/internal/app"
	"mu/internal/service"
)

// The tool set a guest run is given excludes everything account-scoped. This is
// the security property: a guest has no account, so a run that could reach
// mail, files or the wallet would be reaching into somebody's — whichever
// account the tool decided to use, or none, and neither is acceptable.
func TestAGuestRunCannotReachAccountScopedServices(t *testing.T) {
	// Two services, one of each kind, because the registry in a test binary is
	// whatever the test put in it — asserting over an empty one passes while
	// proving nothing, which is how a filter like this stops being checked.
	for _, spec := range []service.Spec{
		{Name: "guestpublic", Handler: new(AgentProbe), Page: "/guestpublic",
			Endpoints: map[string]service.Endpoint{"List": {Doc: "probe"}}},
		{Name: "guestprivate", Handler: new(AgentProbe), Page: "/guestprivate", Scoped: true,
			Endpoints: map[string]service.Endpoint{"List": {Doc: "probe"}}},
	} {
		if err := service.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}

	public := nativeServices(true)
	all := nativeServices(false)

	if !has(public, "guestpublic") {
		t.Error("a guest is not offered a public service, so the answer can only " +
			"ever be talk")
	}
	if has(public, "guestprivate") {
		t.Error("a guest run is offered an account-scoped service — it has no " +
			"account, so that reaches into somebody's or nobody's")
	}
	if !has(all, "guestprivate") {
		t.Error("the full set has lost the scoped service, so this comparison " +
			"proves nothing")
	}
	// The policy itself, not this list: anything scoped is absent, whatever
	// else is registered by the time this runs.
	for _, name := range public {
		if service.AccountScoped(name) {
			t.Errorf("a guest run is offered %q, which is account-scoped", name)
		}
	}
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// The limiter is what stops a stranger spending the operator's money without
// end. It already existed, tested, wired to nothing on this path.
func TestGuestAsksAreRateLimitedPerAddress(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "2")
	const ip = "198.51.100.44"

	if !app.GuestCallAllowed(ip) || !app.GuestCallAllowed(ip) {
		t.Fatal("the first two asks from an address were refused")
	}
	if app.GuestCallAllowed(ip) {
		t.Error("a third ask was allowed past the limit, so an instance can be " +
			"billed without bound by one visitor")
	}
	// Localhost is never limited: a self-hosted instance is one person on their
	// own machine, and rationing them against their own server is absurd.
	for i := 0; i < 5; i++ {
		if !app.GuestCallAllowed("127.0.0.1") {
			t.Fatal("localhost was rate limited")
		}
	}
}

// And the handler asks for a guest before it refuses one. Read from the source
// because the alternative is standing up a model to prove a branch was taken.
func TestTheHandlerOffersAGuestPathBeforeRefusing(t *testing.T) {
	src := readSource(t, "agent.go")
	if !strings.Contains(src, "app.GuestCallAllowed(app.ClientIP(r))") {
		t.Fatal("nothing lets a guest through, so the front page still answers " +
			"every question with a sign-in")
	}
	// A guest writes nothing down.
	if !strings.Contains(src, "if !guest {") {
		t.Error("the record is written for a guest, and a row keyed on an empty " +
			"account is one nothing can list or delete")
	}
	if !strings.Contains(src, "QueryOpts{Public: guest}") {
		t.Error("a guest run is not marked public, so it is given the full tool set")
	}
}
