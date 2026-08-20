package agent

// Which agent answers must not depend on how you arrived.
//
// The router lived inside QueryWithOpts, and the web's streaming handler never
// went through it — so the same question got a specialist on mail
// and the generalist on the web, and nobody noticed because both answers were
// plausible. The fix is that routing sets the options rather than diverting
// into a pipeline of its own, which is also what lets a routed question stream.

import (
	"os"
	"strings"
	"testing"
)

func TestAChosenAgentIsNotRerouted(t *testing.T) {
	chosen := QueryOpts{System: "You are the Markets specialist."}
	_, got := Routed("what is in the news today", chosen)
	if got.System != chosen.System {
		t.Error("the router overrode a system prompt the caller supplied, so the " +
			"agent somebody addressed loses to whatever the words look like")
	}
}

// Routing a question equips the run rather than replacing it.
//
// Addressed directly, because that path is keyword-only and so decides the same
// way every time. The keyword router may consult a model, which a test cannot
// depend on.
func TestRoutingSetsTheOptionsItPicked(t *testing.T) {
	prompt, got := Routed("@markets what is bitcoin trading at?", QueryOpts{})

	if strings.HasPrefix(prompt, "@markets") {
		t.Error("the address was left in the prompt, so the agent is asked to " +
			"answer a question addressed to itself")
	}
	if got.System == "" {
		t.Fatal("addressing an agent directly did not equip the run with its prompt")
	}
	if len(got.Tools) == 0 {
		t.Error("a specialist was chosen with no tool allow-list, so it is the " +
			"catch-all wearing a name")
	}
	if !strings.Contains(strings.Join(got.Tools, " "), "markets") {
		t.Errorf("tools are %v, none of which is a markets tool", got.Tools)
	}
}

func TestTheDefaultAgentIsNotTreatedAsASpecialist(t *testing.T) {
	// The catch-all is what "no specialist" means; equipping it with its own
	// system prompt would make every query look chosen and disable routing for
	// everything downstream.
	if o := PlatformOpts(Platform(DefaultPlatformAgent)); len(o.Tools) != 0 {
		t.Error("the catch-all has a tool allow-list")
	}
	_, got := Routed("hello", QueryOpts{})
	if got.System != "" && len(got.Tools) == 0 {
		t.Error("a plain greeting was equipped as a specialist with no tools")
	}
}

// Every path takes the decision, and takes it in one place.
func TestBothPathsRouteTheSameWay(t *testing.T) {
	b, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if strings.Contains(src, "micro.Orchestrate(") {
		t.Error("a routed question is diverted into Orchestrate again — it takes no " +
			"system prompt and cannot stream, which is what made the web skip " +
			"routing in the first place")
	}
	if n := strings.Count(src, "Routed("); n < 3 {
		t.Errorf("Routed appears %d times: it should be declared once and called by "+
			"both QueryWithOpts and the streaming handler", n)
	}
	if !strings.Contains(src, "routedPrompt, nopts := Routed(req.Prompt, nopts)") {
		t.Error("the web's streaming handler does not route, so it answers as the " +
			"generalist while every other client gets a specialist")
	}
}
