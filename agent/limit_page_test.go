package agent

// A limit you meet by doing the work first is the wrong kind of limit.
//
// The agent cap was checked only where an agent is actually made, so finding
// out you had reached it meant pressing New agent, filling in a name, a
// description, a prompt and a set of tools, submitting, and getting an alert
// telling you to go somewhere else. Everything in that sentence before the
// alert was wasted.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

// capped stands up an account with a plan that allows n agents, and gives it
// one more than it should have when over is true.
func capped(t *testing.T, id string, allowed, made int) {
	t.Helper()
	if _, err := auth.GetAccount(id); err != nil {
		if err := auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}); err != nil {
			t.Skipf("cannot create an account here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	}

	orig := AgentAllowance
	AgentAllowance = func(string) int { return allowed }
	t.Cleanup(func() { AgentAllowance = orig })

	for i := 0; i < made; i++ {
		a := &Agent{Owner: id, Name: "agent", Kind: External}
		if _, _, err := CreateAgent(id, a.Name, a.Kind, "you are helpful", "", nil, false); err != nil {
			t.Fatalf("making agent %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, a := range Agents(id) {
			RemoveAgent(id, a.ID) //nolint:errcheck
		}
	})
}

func TestAtTheLimitTheAnswerIsKnownBeforeTheFormIsShown(t *testing.T) {
	const owner = "agent-limit-full"
	capped(t, owner, 1, 1)

	full, have, max := AtAgentLimit(owner)
	if !full {
		t.Fatalf("an account with %d of %d agents is not reported as full", have, max)
	}
	if have != 1 || max != 1 {
		t.Errorf("AtAgentLimit = %v, %d, %d", full, have, max)
	}
}

func TestBelowTheLimitNothingIsInTheWay(t *testing.T) {
	const owner = "agent-limit-room"
	capped(t, owner, 5, 1)

	if full, have, max := AtAgentLimit(owner); full {
		t.Errorf("an account with %d of %d agents is being refused", have, max)
	}
}

// An unlimited plan is not a limit of zero. agentAllowance answers 0 when
// nothing knows about plans, and a naive check would read that as "no agents
// allowed" and lock everybody out of the builder.
func TestNoAllowanceMachineryMeansNoLimitRatherThanNoAgents(t *testing.T) {
	const owner = "agent-limit-noplan"
	orig := AgentAllowance
	AgentAllowance = nil
	t.Cleanup(func() { AgentAllowance = orig })

	if full, _, _ := AtAgentLimit(owner); full {
		t.Error("with no allowance machinery every account is treated as full")
	}
}

// The builder does not draw a form somebody cannot submit.
func TestTheBuilderRefusesBeforeAskingForAnything(t *testing.T) {
	const owner = "agent-limit-builder"
	capped(t, owner, 1, 1)

	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Skipf("cannot sign in here: %v", err)
	}
	r := httptest.NewRequest("GET", "/agent/new", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	rr := httptest.NewRecorder()
	NewAgentHandler(rr, r)

	body := rr.Body.String()
	if strings.Contains(body, `id="b-prompt"`) {
		t.Error("the builder drew its form to an account that cannot submit it")
	}
	if !strings.Contains(body, "/account") {
		t.Error("the refusal does not offer the thing that would change the answer")
	}
}
