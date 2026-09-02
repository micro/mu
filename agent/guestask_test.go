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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/app"
	"mu/internal/service"
)

// What a guest run can reach, and what it must not.
//
// Two properties, and the second is the security one.
//
// The set is the front door's, named in internal/service. It used to be
// "everything not account-scoped", which is two dozen services and all their
// methods in the prompt of every stranger's question — slow, and paid for per
// question by whoever runs the instance.
//
// And nothing account-scoped is in it, whatever else is registered. A guest has
// no account, so a run that could reach mail, files or the wallet would be
// reaching into somebody's — whichever account the tool decided to use, or none,
// and neither is acceptable.
func TestAGuestRunGetsTheFrontDoorSetAndNothingScoped(t *testing.T) {
	// One of each kind, because the registry in a test binary is whatever the
	// test put in it — asserting over an empty one passes while proving
	// nothing, which is how a filter like this stops being checked.
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

	// A public service that is not on the front door's list is not offered
	// either. Being harmless is not the same as being worth a slot in every
	// stranger's prompt.
	if has(public, "guestpublic") {
		t.Error("a guest is offered a service that is not in the front door's set, " +
			"so the set is not being applied at all")
	}
	if has(public, "guestprivate") {
		t.Error("a guest run is offered an account-scoped service — it has no " +
			"account, so that reaches into somebody's or nobody's")
	}
	if !has(all, "guestprivate") {
		t.Error("the full set has lost the scoped service, so this comparison " +
			"proves nothing")
	}
	if len(public) >= len(all) {
		t.Errorf("a guest is offered %d services and a signed-in caller %d — "+
			"the guest set is meant to be the smaller one, and the whole reason "+
			"it exists is speed", len(public), len(all))
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
func TestGuestAsksAreRateLimited(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "2")

	ask := func() bool {
		r := httptest.NewRequest(http.MethodGet, "/agent", nil)
		r.RemoteAddr = "198.51.100.44:1"
		return app.GuestAllowed(r)
	}
	if !ask() || !ask() {
		t.Fatal("the first two asks from an address were refused")
	}
	if ask() {
		t.Error("a third ask was allowed past the limit, so an instance can be " +
			"billed without bound by one visitor")
	}
	// Localhost is never limited: a self-hosted instance is one person on their
	// own machine, and rationing them against their own server is absurd.
	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodGet, "/agent", nil)
		r.RemoteAddr = "127.0.0.1:1"
		if !app.GuestAllowed(r) {
			t.Fatal("localhost was rate limited")
		}
	}
}

// And the handler asks for a guest before it refuses one. Read from the source
// because the alternative is standing up a model to prove a branch was taken.
func TestTheHandlerOffersAGuestPathBeforeRefusing(t *testing.T) {
	src := readSource(t, "agent.go")
	if !strings.Contains(src, "app.GuestAllowed(r)") {
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

// A guest gets the quick model, not the thorough one.
//
// Both currencies a better model spends are the wrong ones on the front door.
// Somebody who arrived to find one thing out is waiting, and somebody who wants
// a model to work a problem through has signed in and gone to /agent. It is
// also what makes leaving the door open affordable: the quick end of every
// provider here is one to two orders of magnitude cheaper, and what a stranger
// costs is the whole reason a guest allowance exists.
func TestAGuestGetsTheQuickModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ATLASCLOUD_API_KEY", "test-key")
	t.Setenv("GUEST_MODEL", "")

	_, _, guestModel, _, ok := nativeLLMFor("", true)
	if !ok {
		t.Skip("no provider resolves in this build")
	}
	_, _, userModel, _, _ := nativeLLMFor("", false)

	if guestModel == userModel {
		t.Errorf("a guest and a signed-in account get the same model (%q).\n"+
			"The front door is meant to be fast and cheap; the thorough one is\n"+
			"what signing in gets you.", guestModel)
	}
}

// And an operator who names one is not second-guessed.
func TestAnOperatorCanNameTheGuestModel(t *testing.T) {
	t.Setenv("ATLASCLOUD_API_KEY", "test-key")
	t.Setenv("GUEST_MODEL", "deepseek-ai/DeepSeek-V3")

	_, _, model, _, ok := nativeLLMFor("", true)
	if !ok {
		t.Skip("no provider resolves in this build")
	}
	if model != "deepseek-ai/DeepSeek-V3" {
		t.Errorf("GUEST_MODEL was ignored: got %q", model)
	}
}

// The guest path asks for it. Read from the source, because proving the branch
// was taken otherwise means standing up a model.
func TestTheGuestRunIsMarkedPublic(t *testing.T) {
	src := readSource(t, "native.go")
	if !strings.Contains(src, "nativeLLMFor(opts.Model, opts.Public)") {
		t.Error("the model choice does not know whether the caller is a guest, so\n" +
			"every stranger's question runs on the expensive model")
	}
}
