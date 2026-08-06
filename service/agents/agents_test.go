package agents

import (
	"context"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/service"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-agents-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// AgentProbe stands in for a real service so the scope tests do not depend on
// which service packages happen to be linked into this binary.
type AgentProbe struct{}

func (AgentProbe) List(ctx context.Context, req *struct{}, rsp *struct {
	Text string `json:"text"`
}) error {
	return nil
}

func owner(t *testing.T, id string) string {
	t.Helper()
	if err := auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	return id
}

func probes(t *testing.T) {
	t.Helper()
	for _, name := range []string{"probealpha", "probebeta"} {
		if _, known := service.SpecFor(name); known {
			continue
		}
		if err := service.Register(service.Spec{
			Name: name, Handler: new(AgentProbe), Page: "/" + name, Scoped: true,
			Endpoints: map[string]service.Endpoint{"List": {Doc: "probe"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// The whole point: the scope chosen here is written into the token, so the
// confinement travels with the credential rather than living only on a page.
func TestAnAgentsScopeIsWrittenIntoItsToken(t *testing.T) {
	probes(t)
	id := owner(t, "agentowner")

	a, secret, err := Create(id, "Reader", External, "reads things", []string{"probealpha"})
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Fatal("no token secret was returned")
	}

	tok, err := auth.GetTokenByID(a.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	if !tok.Scoped() {
		t.Fatal("the agent's token is unscoped")
	}
	if !tok.AllowsService("probealpha") || tok.AllowsService("probebeta") {
		t.Errorf("the token allows %v, want probealpha only", tok.Services())
	}
}

// A scope naming something this instance does not run would allow nothing and
// look like it allowed something.
func TestAScopeCannotNameAServiceThatDoesNotExist(t *testing.T) {
	probes(t)
	id := owner(t, "agentowner2")

	a, _, err := Create(id, "Confused", External, "", []string{"probealpha", "nosuchservice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Services) != 1 || a.Services[0] != "probealpha" {
		t.Errorf("scope is %v, want the invented service dropped", a.Services)
	}
}

// Choosing nothing is the old behaviour of a bare token. It must be possible —
// and it must be visible as "everything", not quietly presented as a scope.
func TestAnAgentWithNoScopeChosenIsUnscoped(t *testing.T) {
	id := owner(t, "agentowner3")

	a, _, err := Create(id, "Everything", External, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Unscoped() {
		t.Error("an agent created with no services reads as scoped")
	}
	tok, err := auth.GetTokenByID(a.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Scoped() {
		t.Error("a token issued with no scope reads as scoped")
	}
}

// Removing an agent must revoke its credential. Gone from the list but still
// able to call is the worst of both.
func TestRemovingAnAgentRevokesItsToken(t *testing.T) {
	probes(t)
	id := owner(t, "agentowner4")

	a, _, err := Create(id, "Temporary", External, "", []string{"probealpha"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove(id, a.ID); err != nil {
		t.Fatal(err)
	}
	if Get(id, a.ID) != nil {
		t.Error("the agent survived removal")
	}
	if _, err := auth.GetTokenByID(a.TokenID); err == nil {
		t.Error("the agent's token still exists after removal")
	}
}

// One person's agents are their own.
func TestAgentsAreOwnedByOneAccount(t *testing.T) {
	probes(t)
	mine := owner(t, "agentmine")
	theirs := owner(t, "agenttheirs")

	a, _, err := Create(mine, "Mine", External, "", []string{"probealpha"})
	if err != nil {
		t.Fatal(err)
	}
	if Get(theirs, a.ID) != nil {
		t.Error("another account could read my agent")
	}
	if err := Remove(theirs, a.ID); err == nil {
		t.Error("another account could delete my agent")
	}
	if Get(mine, a.ID) == nil {
		t.Error("the owner lost their own agent")
	}
}

// The endpoint an agent is pointed at carries its scope, so the tool list it
// reads every turn is its own rather than the whole instance.
func TestTheEndpointCarriesTheScope(t *testing.T) {
	probes(t)
	id := owner(t, "agentowner5")

	scoped, _, err := Create(id, "Narrow", External, "", []string{"probealpha"})
	if err != nil {
		t.Fatal(err)
	}
	if got := scoped.Endpoint("https://example.test"); !strings.Contains(got, "?tools=probealpha") {
		t.Errorf("endpoint is %q, want the scope in it", got)
	}

	wide, _, err := Create(id, "Wide", External, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := wide.Endpoint("https://example.test"); strings.Contains(got, "?tools=") {
		t.Errorf("an unscoped agent got a scoped endpoint: %q", got)
	}
}
