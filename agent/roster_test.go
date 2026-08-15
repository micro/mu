package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"mu/agent/micro"
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

	a, secret, err := CreateAgent(id, "Reader", External, "reads things", "", []string{"probealpha"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Fatal("no token secret was returned")
	}

	tok, err := auth.TokenByID(a.TokenID)
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

	a, _, err := CreateAgent(id, "Confused", External, "", "", []string{"probealpha", "nosuchservice"}, true)
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

	a, _, err := CreateAgent(id, "Everything", External, "", "", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Unscoped() {
		t.Error("an agent created with no services reads as scoped")
	}
	tok, err := auth.TokenByID(a.TokenID)
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

	a, _, err := CreateAgent(id, "Temporary", External, "", "", []string{"probealpha"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveAgent(id, a.ID); err != nil {
		t.Fatal(err)
	}
	if For(id, a.ID) != nil {
		t.Error("the agent survived removal")
	}
	if _, err := auth.TokenByID(a.TokenID); err == nil {
		t.Error("the agent's token still exists after removal")
	}
}

// One person's agents are their own.
func TestAgentsAreOwnedByOneAccount(t *testing.T) {
	probes(t)
	mine := owner(t, "agentmine")
	theirs := owner(t, "agenttheirs")

	a, _, err := CreateAgent(mine, "Mine", External, "", "", []string{"probealpha"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if For(theirs, a.ID) != nil {
		t.Error("another account could read my agent")
	}
	if err := RemoveAgent(theirs, a.ID); err == nil {
		t.Error("another account could delete my agent")
	}
	if For(mine, a.ID) == nil {
		t.Error("the owner lost their own agent")
	}
}

// The endpoint an agent is pointed at carries its scope, so the tool list it
// reads every turn is its own rather than the whole instance.
func TestTheEndpointCarriesTheScope(t *testing.T) {
	probes(t)
	id := owner(t, "agentowner5")

	scoped, _, err := CreateAgent(id, "Narrow", External, "", "", []string{"probealpha"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := scoped.Endpoint("https://example.test"); !strings.Contains(got, "?tools=probealpha") {
		t.Errorf("endpoint is %q, want the scope in it", got)
	}

	wide, _, err := CreateAgent(id, "Wide", External, "", "", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := wide.Endpoint("https://example.test"); strings.Contains(got, "?tools=") {
		t.Errorf("an unscoped agent got a scoped endpoint: %q", got)
	}
}

// One store, one list. Agents made in the chat used to live in agent/micro's
// own file while /agents wrote here, so "my agents" depended on which page you
// asked. Existing ones are imported — keeping their name, prompt and tools, and
// getting no token, because nobody asked for a credential and minting one on
// somebody's behalf is a decision rather than an import.
func TestImportingTheOldStoreKeepsAgentsAndMintsNoTokens(t *testing.T) {
	probes(t)
	id := owner(t, "importer")

	n := ImportUserAgents(map[string][]*micro.Agent{
		id: {{ID: "u_old", Name: "Legacy", Description: "an old one",
			SystemPrompt: "be helpful", Tools: []string{"probealpha"}}},
	})
	if n != 1 {
		t.Fatalf("imported %d, want 1", n)
	}

	var found *Agent
	for _, a := range Agents(id) {
		if a.Name == "Legacy" {
			found = a
		}
	}
	if found == nil {
		t.Fatal("the imported agent is not in the roster")
	}
	if found.Prompt != "be helpful" || found.Description != "an old one" {
		t.Errorf("import lost detail: %+v", found)
	}
	if found.TokenID != "" {
		t.Error("import minted a credential nobody asked for")
	}
	if len(found.Services) != 1 || found.Services[0] != "probealpha" {
		t.Errorf("import lost the tool set: %v", found.Services)
	}

	// Running twice must not double the list.
	if again := ImportUserAgents(map[string][]*micro.Agent{
		id: {{ID: "u_old", Name: "Legacy", SystemPrompt: "be helpful"}},
	}); again != 0 {
		t.Errorf("a second import added %d more", again)
	}
}

// An agent without a credential can be given one later, because an agent you
// only talked to may need to run somewhere else.
func TestATokenlessAgentCanBeIssuedOne(t *testing.T) {
	probes(t)
	id := owner(t, "issuer")

	a, secret, err := CreateAgent(id, "Quiet", Hosted, "think", "", []string{"probealpha"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "" || a.TokenID != "" {
		t.Fatal("an agent created without a token got one")
	}

	issued, err := IssueToken(id, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued == "" {
		t.Error("no secret was returned")
	}
	got := For(id, a.ID)
	if got.TokenID == "" {
		t.Error("the issued token was not recorded")
	}
	tok, err := auth.TokenByID(got.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	// The scope travels with the credential, exactly as it does at creation.
	if !tok.AllowsService("probealpha") || tok.AllowsService("probebeta") {
		t.Errorf("the issued token allows %v", tok.Services())
	}
}

// Which kind gets a credential is the whole difference between the two options
// on the create form. Both used to get one, which made the choice cosmetic: you
// could say "runs here" and still be handed a token and an MCP endpoint for
// something nothing outside was ever going to call, and the row's "Issue token"
// action — written for exactly that case — could never appear.
func TestOnlyAnAgentThatRunsElsewhereIsGivenAToken(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want bool
		why  string
	}{
		{External, true, "it cannot be called without one"},
		{Hosted, false, "nothing outside it needs to call it"},
		{"", true, "the form defaults to external, and an agent that cannot be called is the worse failure"},
	} {
		if got := issuesToken(tc.kind); got != tc.want {
			t.Errorf("issuesToken(%q) = %v, want %v — %s", tc.kind, got, tc.want, tc.why)
		}
	}
}
