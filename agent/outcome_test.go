package agent

// How a run went, which nothing in this product could say.

import (
	"context"
	"os"
	"strings"
	"testing"

	gmagent "go-micro.dev/v6/agent"
	"go-micro.dev/v6/store"
)

// A run that happened is a run the operator can see the shape of.
//
// The timeline was already being loaded after every question — recordRunCost
// reads it — and only the tokens were taken off it. So the instance could say
// what it had spent and not whether the answers were arriving.
func TestARunsShapeIsRecorded(t *testing.T) {
	ForgetOutcomes()

	runs := store.NewMemoryStore()
	a := gmagent.New(
		gmagent.Name("outcome-test-agent"),
		gmagent.Provider("costtest"),
		gmagent.Model("claude-sonnet-5"),
		gmagent.WithStore(runs),
	)
	defer a.Stop()
	if _, err := a.Ask(context.Background(), "how did this go"); err != nil {
		t.Fatal(err)
	}
	recordRunCost(runs, "outcome-test-agent", "agent")

	got := Outcomes()
	if len(got) != 1 {
		t.Fatalf("a run finished and %d outcomes were recorded", len(got))
	}
	if got[0].Steps < 1 {
		t.Errorf("a run that called a model recorded %d steps", got[0].Steps)
	}
	if got[0].Status != "done" {
		t.Errorf("a run that answered is %q", got[0].Status)
	}
	if got[0].Agent != "outcome-test-agent" || got[0].Caller != "agent" {
		t.Errorf("recorded against %q/%q", got[0].Agent, got[0].Caller)
	}
}

// The two lists that turn "make it reliable" into work: what gets called, and
// what goes wrong.
func TestTheSummaryNamesWhatBreaks(t *testing.T) {
	ForgetOutcomes()

	recordOutcome([]gmagent.RunEvent{
		{Kind: "model", Attempt: 1, LatencyMS: 400},
		{Kind: "tool", Name: "news_headlines"},
		{Kind: "tool", Name: "web_search", Error: "brave: 429"},
		{Kind: "model", Attempt: 2, LatencyMS: 900},
	}, "micro", "agent")
	recordOutcome([]gmagent.RunEvent{
		{Kind: "model", Attempt: 1},
		{Kind: "tool", Name: "news_headlines"},
		{Kind: "tool", Name: "web_search", Error: "brave: 429"},
	}, "micro", "agent")

	h := Summary()
	if h.Runs != 2 {
		t.Fatalf("summary over %d runs", h.Runs)
	}
	if h.ToolErrors != 2 {
		t.Errorf("%d tool errors, want 2", h.ToolErrors)
	}
	if h.Retried != 1 {
		t.Errorf("%d runs retried, want 1 — a run that answers on the third "+
			"attempt looks perfect from outside", h.Retried)
	}
	if len(h.TopTools) == 0 || h.TopTools[0].N != 2 {
		t.Errorf("the tools actually called are not counted: %#v", h.TopTools)
	}
	// The evidence for or against narrowing the catalogue is exactly this list.
	names := map[string]bool{}
	for _, c := range h.TopTools {
		names[c.Name] = true
	}
	if !names["news_headlines"] || !names["web_search"] {
		t.Errorf("a called tool is missing from the counts: %#v", h.TopTools)
	}
	if len(h.TopErrors) != 1 || h.TopErrors[0].Name != "brave: 429" || h.TopErrors[0].N != 2 {
		t.Errorf("the thing that keeps failing is not named: %#v", h.TopErrors)
	}
}

// A run that did not finish is not counted as one that did.
func TestAFailedRunIsNotDone(t *testing.T) {
	ForgetOutcomes()
	recordOutcome([]gmagent.RunEvent{
		{Kind: "model", Status: "timeout", Error: "context deadline exceeded"},
	}, "micro", "agent")

	if h := Summary(); h.Failed != 1 {
		t.Errorf("%d of 1 runs counted as failed", h.Failed)
	}
	if got := Outcomes(); got[0].Status != "timeout" {
		t.Errorf("status recorded as %q", got[0].Status)
	}
}

// Nothing here says whose run it was.
//
// The record is bounded, in memory and an operator's view of the machine. The
// history is the conversation, in internal/thread, and it belongs to whoever
// had it — see the package comment.
func TestTheRecordIsBoundedAndImpersonal(t *testing.T) {
	ForgetOutcomes()
	for i := 0; i < keptRuns+25; i++ {
		recordOutcome([]gmagent.RunEvent{{Kind: "model"}}, "micro", "agent")
	}
	if n := len(Outcomes()); n > keptRuns {
		t.Errorf("holding %d runs, want at most %d", n, keptRuns)
	}
}

// And the record has a reader.
//
// A bounded ring nobody draws is the pattern this codebase keeps catching: code
// that compiles, runs, costs memory and answers no question. The whole point of
// recording how runs go is that an operator can look.
func TestTheRecordIsDrawnSomewhere(t *testing.T) {
	b, err := os.ReadFile("../admin/traffic.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "agentHealthCard()") {
		t.Error("nothing renders the run record, so it is a ring buffer costing " +
			"memory and answering nobody")
	}
}
