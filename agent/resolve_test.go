package agent

// An agent you built has to be the agent that answers.
//
// The ask path looked a named agent up in agent/micro's user store — the store
// the roster replaced — so every agent made since /agents existed was found by
// nothing: the lookup returned nil, the system prompt and tool scope were
// dropped, and the default assistant answered in its place without saying so.
// You could name an agent, give it a voice and a scope, ask it a question, and
// get Micro back.

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// resolveProbe is a service registered by this test. Naming a real one would
// make the assertion depend on which service packages happen to be linked into
// this test binary — validServices drops anything unregistered, so the scope
// would come back empty for a reason that has nothing to do with resolution.
type ResolveProbe struct{}

func (ResolveProbe) List(ctx context.Context, req *struct{}, rsp *struct {
	Text string `json:"text"`
}) error {
	return nil
}

func registerResolveProbe(t *testing.T) string {
	t.Helper()
	const name = "resolveprobe"
	if _, known := service.SpecFor(name); known {
		return name
	}
	if err := service.Register(service.Spec{
		Name: name, Handler: new(ResolveProbe), Page: "/" + name,
		Endpoints: map[string]service.Endpoint{"List": {Doc: "probe"}},
	}); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestAnAgentBuiltHereIsTheAgentThatAnswers(t *testing.T) {
	const owner = "resolve_owner"
	const prompt = "You are a pirate. Always begin your reply with AHOY."

	svc := registerResolveProbe(t)
	a, _, err := CreateAgent(owner, "Pirate", Hosted, prompt, "", []string{svc}, false)
	if err != nil {
		t.Skipf("cannot create an agent in this environment: %v", err)
	}
	t.Cleanup(func() { _ = RemoveAgent(owner, a.ID) })

	got := resolveAgent(owner, a.ID)
	if got == nil {
		t.Fatal("an agent in the roster was not found by the path that answers questions")
	}
	if !strings.Contains(got.SystemPrompt, "AHOY") {
		t.Fatalf("the agent's own prompt was not carried through: %q", got.SystemPrompt)
	}
	if len(got.Tools) != 1 || got.Tools[0] != svc {
		t.Fatalf("the agent's tool scope was not carried through: %v", got.Tools)
	}
}

// Resolution is per account. Somebody else's agent id is not yours to run, and
// asking as one must fall back to the default rather than reaching across.
func TestAnotherAccountsAgentDoesNotResolve(t *testing.T) {
	const owner, stranger = "resolve_mine", "resolve_theirs"
	a, _, err := CreateAgent(owner, "Private", Hosted, "You are mine alone.", "", nil, false)
	if err != nil {
		t.Skipf("cannot create an agent in this environment: %v", err)
	}
	t.Cleanup(func() { _ = RemoveAgent(owner, a.ID) })

	if got := resolveAgent(stranger, a.ID); got != nil {
		t.Fatalf("a stranger resolved somebody else's agent: %+v", got)
	}
	if got := resolveAgent(owner, ""); got != nil {
		t.Fatal("an empty id resolved to something")
	}
}
