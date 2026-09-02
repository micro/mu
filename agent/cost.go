package agent

// What a run cost.
//
// # The hole this fills
//
// mu priced every model call it made through internal/ai and none of the ones
// the agent made, because the agent does not make them through internal/ai —
// go-micro holds the loop and calls the provider itself. Every other cost in
// the product was recorded: Google's places calls, Brave's searches, the
// summariser, the brief. The agent, which is the product, was free.
//
// The number that says how bad it was: production believed it had spent
// sixty-seven cents since March.
//
// # Why this reads a timeline rather than counting as it goes
//
// go-micro's agent already records what each model call used — provider, model
// and tokens — as a run timeline, whether or not tracing is configured. The
// tokens were there the whole time; nothing read them.
//
// So this reads them. After the run, from the run's own store, which is also
// the fix for a second thing that timeline was doing: it was being written to
// the shared bbolt store under a per-request agent name, so every question
// anybody asked left a database file behind, on disk, forever, that nothing
// ever opened. A run store that lives for the run is where a run's timeline
// belongs, and it turns a disk write on the answer path into a map write.
//
// # What it does not see
//
// A delegated sub-agent runs under its own name and files its timeline under
// that name, and the summaries here are looked up by the name of the agent that
// was built. Nothing in mu delegates today. If something does, its model calls
// will be unpriced in the same way this file exists to fix, so this comment is
// the warning.

import (
	"sort"
	"sync"

	gmagent "go-micro.dev/v6/agent"
	"go-micro.dev/v6/store"

	"mu/internal/ai"
	"mu/internal/app"
)

// costCaller is what the spend log files a run under.
//
// Two names, split on who asked. A signed-out visitor is answered by the quick
// end of the provider and a signed-in account by the thorough end — see
// nativeLLMFor — so they are different questions at different prices, and the
// one an operator needs to be able to read on its own is the front door: what a
// stranger costs is the whole argument for a guest allowance, and it cannot be
// argued from a total that has both in it.
func costCaller(opts QueryOpts) string {
	if opts.Public {
		return "agent_guest"
	}
	return "agent"
}

// recordRunCost prices the run that just finished and records what it spent.
//
// caller is what the spend log files it under: the name of the agent that ran,
// so a bill can be read by which agent earned it.
func recordRunCost(st store.Store, agentName, caller string) {
	if st == nil || agentName == "" {
		return
	}
	summaries, err := gmagent.ListRunSummaries(st, agentName)
	if err != nil {
		return
	}
	for _, s := range summaries {
		events, err := gmagent.LoadRunEvents(st, agentName, s.RunID)
		if err != nil {
			continue
		}
		for _, m := range spendByModel(events) {
			if m.input == 0 && m.output == 0 {
				// A model call that reported no tokens. Priced at zero it would
				// be a row saying a run was free, which is the lie this file
				// exists to stop telling, so it is said out loud once instead.
				noUsageReported.Do(func() {
					app.Log("agent", "the provider reported no token usage for model %q, "+
						"so agent runs on it cannot be priced", m.model)
				})
				continue
			}
			ai.RecordAgentUsage(caller, m.model, m.input, m.output, m.calls)
		}
	}
}

// noUsageReported keeps that warning to once per process rather than once per
// question.
var noUsageReported sync.Once

// modelSpend is one model's share of one run.
type modelSpend struct {
	model  string
	input  int
	output int
	calls  int
}

// spendByModel sums a run's model calls, one entry per model, in a fixed order.
//
// Almost always one entry: a run asks the same model every step. It is a map
// anyway because a run that falls back to a second provider mid-way is a run
// with two prices, and pricing that at either one of them would be wrong in
// whichever direction the fallback went.
func spendByModel(events []gmagent.RunEvent) []modelSpend {
	byModel := map[string]*modelSpend{}
	for _, e := range events {
		if e.Kind != "model" && e.Kind != "stream" {
			continue
		}
		m, ok := byModel[e.Model]
		if !ok {
			m = &modelSpend{model: e.Model}
			byModel[e.Model] = m
		}
		m.input += e.Tokens.InputTokens
		m.output += e.Tokens.OutputTokens
		m.calls++
	}

	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]modelSpend, 0, len(names))
	for _, name := range names {
		out = append(out, *byModel[name])
	}
	return out
}
