package agent

// What a run cost, and the two things that were wrong before it: nothing
// recorded it, and the record it could have been read from was being kept
// forever on disk.

import (
	"context"
	"strings"
	"testing"

	gmagent "go-micro.dev/v6/agent"
	gmai "go-micro.dev/v6/ai"
	"go-micro.dev/v6/store"

	"mu/internal/app"
	"mu/internal/service"
)

// costTestModel answers once, reporting the tokens it used.
type costTestModel struct{ opts gmai.Options }

func (m *costTestModel) Init(opts ...gmai.Option) error {
	for _, o := range opts {
		o(&m.opts)
	}
	return nil
}
func (m *costTestModel) Options() gmai.Options { return m.opts }
func (m *costTestModel) String() string        { return "costtest" }
func (m *costTestModel) Stream(context.Context, *gmai.Request, ...gmai.GenerateOption) (gmai.Stream, error) {
	return nil, nil
}
func (m *costTestModel) Generate(context.Context, *gmai.Request, ...gmai.GenerateOption) (*gmai.Response, error) {
	return &gmai.Response{
		Reply: "answered",
		Usage: gmai.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
	}, nil
}

func init() {
	gmai.Register("costtest", func(opts ...gmai.Option) gmai.Model {
		return &costTestModel{opts: gmai.NewOptions(opts...)}
	})
}

// A run that happened is a run that shows up in the spend log.
//
// It did not. recordUsage is reached from ai.Ask and the agent does not go
// through ai.Ask, so the only path that answers most questions — and the one
// that can make forty model calls answering one of them — was recorded as free.
// The visible symptom was a production instance reporting sixty-seven cents of
// lifetime spend, all of it Google and Brave.
func TestARunIsPricedFromItsOwnTimeline(t *testing.T) {
	before := len(app.GetUsageSummary().RecentCalls)

	runs := store.NewMemoryStore()
	a := gmagent.New(
		gmagent.Name("cost-test-agent"),
		gmagent.Provider("costtest"),
		// A model with a known price, so the arithmetic below is checkable:
		// 100 input at $2/M is 0.02c, 50 output at $10/M is 0.05c.
		gmagent.Model("claude-sonnet-5"),
		gmagent.WithStore(runs),
	)
	defer a.Stop()
	if _, err := a.Ask(context.Background(), "what did this cost"); err != nil {
		t.Fatal(err)
	}

	recordRunCost(runs, "cost-test-agent", "agent")

	recent := app.GetUsageSummary().RecentCalls
	if len(recent) <= before {
		t.Fatal("a run finished and the spend log did not gain a record")
	}
	got := recent[0] // newest first
	if got.Caller != "agent" {
		t.Errorf("recorded against caller %q, want the agent", got.Caller)
	}
	if got.Service != "claude" {
		t.Errorf("recorded against service %q, want the model's provider", got.Service)
	}
	// The number is the point. A record with the right shape and no money in it
	// is the bug this replaces.
	if want := 0.07; got.CostCents < want-0.0001 || got.CostCents > want+0.0001 {
		t.Errorf("run cost %.4fc, want %.4fc — 100 input and 50 output at Sonnet 5's price", got.CostCents, want)
	}
	if got.Details["input_tokens"] != 100 || got.Details["output_tokens"] != 50 {
		t.Errorf("the record does not carry the tokens: %#v", got.Details)
	}
}

// A run's timeline is not kept.
//
// go-micro writes one — every model call with its tokens — to whatever store
// the agent is given, and the agent name is fresh per request. Given the shared
// bbolt store, that is a database file per question asked, in ~/.mu/store/agent,
// that nothing ever reads and nothing ever deletes.
func TestARunsTimelineIsNotWrittenToTheSharedStore(t *testing.T) {
	clearProviders(t)
	t.Setenv("AI_PROVIDER", "ollama")
	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434")
	t.Setenv("OPENAI_API_KEY", "local")
	t.Setenv("OPENAI_MODEL", "llama3.2")

	run, built := buildNativeAgent("", "hello", QueryOpts{})
	if !built {
		t.Fatal("no agent built")
	}
	defer run.agent.Stop()

	if run.runs == nil {
		t.Fatal("the run has no store of its own")
	}
	if run.agent.Options().Store != run.runs {
		t.Error("the agent is not writing its timeline to the run's own store")
	}
	if run.agent.Options().Store == service.Store() {
		t.Error("the run timeline is still going to the shared store, one file per question, forever")
	}
	// And it is still findable while the run is alive, because pricing the run
	// is the whole reason it is written.
	if run.name == "" {
		t.Error("the run does not know the name its timeline is filed under")
	}
}

// A run that spends on two models is two prices.
//
// Almost every run is one model. A run that falls back to a second provider
// part-way is not, and summing it under either single name charges some of the
// tokens at the other one's rate.
func TestSpendIsSummedPerModel(t *testing.T) {
	got := spendByModel([]gmagent.RunEvent{
		{Kind: "run"},
		{Kind: "model", Model: "claude-sonnet-5", Tokens: gmai.Usage{InputTokens: 10, OutputTokens: 1}},
		{Kind: "tool", Name: "weather"},
		{Kind: "model", Model: "claude-sonnet-5", Tokens: gmai.Usage{InputTokens: 20, OutputTokens: 2}},
		{Kind: "model", Model: "claude-haiku-4-5", Tokens: gmai.Usage{InputTokens: 5, OutputTokens: 3}},
		{Kind: "done"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d models, want 2: %#v", len(got), got)
	}
	// Sorted, so the same run does not report itself in a different order each
	// time it is priced.
	if got[0].model != "claude-haiku-4-5" || got[1].model != "claude-sonnet-5" {
		t.Fatalf("models are not in a fixed order: %#v", got)
	}
	if got[1].input != 30 || got[1].output != 3 || got[1].calls != 2 {
		t.Errorf("sonnet's share is %#v, want 30 in, 3 out, over 2 calls", got[1])
	}
	if got[0].calls != 1 {
		t.Errorf("haiku's share is %#v, want 1 call", got[0])
	}
}

// A tool call is not a model call. Counting one as the other prices the steps
// that cost nothing.
func TestOnlyModelCallsAreCounted(t *testing.T) {
	got := spendByModel([]gmagent.RunEvent{
		{Kind: "run"},
		{Kind: "tool", Name: "weather"},
		{Kind: "tool", Name: "news"},
		{Kind: "done"},
	})
	if len(got) != 0 {
		t.Errorf("a run with no model calls was priced: %#v", got)
	}
}

// Somebody who is not signed in is answered by a different model at a different
// price, and an operator deciding what a guest may have has to be able to read
// that on its own.
func TestAGuestRunIsFiledSeparately(t *testing.T) {
	if got := costCaller(QueryOpts{Public: true}); got != "agent_guest" {
		t.Errorf("a guest run is filed as %q", got)
	}
	if got := costCaller(QueryOpts{}); got != "agent" {
		t.Errorf("a signed-in run is filed as %q", got)
	}
	if strings.HasPrefix(costCaller(QueryOpts{}), costCaller(QueryOpts{Public: true})) {
		t.Error("one caller name is a prefix of the other, which is how usage totals get read wrong")
	}
}
