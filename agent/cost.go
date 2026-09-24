package agent

// Provider HTTP responses account for spend on supported providers, including
// follow-up calls hidden inside Generate. The run timeline still supplies
// outcomes and is the fallback for providers without a wire meter.

import (
	"sort"
	"sync"

	gmagent "go-micro.dev/v6/agent"
	"go-micro.dev/v6/store"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/usage"
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
func recordRunCost(st store.Store, agentName, caller, account, provider string, meter *ai.CallMeter) {
	runID := agentName
	defer func() { meter.Record(caller, account, runID) }()
	if st == nil || agentName == "" {
		return
	}
	summaries, err := gmagent.ListRunSummaries(st, agentName)
	if err != nil {
		return
	}
	for _, s := range summaries {
		runID = s.RunID
		events, err := gmagent.LoadRunEvents(st, agentName, s.RunID)
		if err != nil {
			continue
		}
		// How it went, as well as what it cost.
		//
		// The timeline is already here and it carries far more than tokens:
		// the status of every step, the error, the latency, the retry count.
		// All of it was being loaded and dropped, which is why nothing in this
		// product could say whether Micro was answering — only what it had
		// spent trying. See outcome.go.
		recordOutcome(events, agentName, caller)
		if meter != nil {
			continue // HTTP responses account for every provider tool round.
		}
		for _, m := range spendByModel(events) {
			if provider == "codex" {
				usage.RecordModels(m.calls)
				app.RecordUsage("codex", caller, 0, map[string]any{"model": m.model, "account": account, "run_id": s.RunID, "input_tokens": m.input, "output_tokens": m.output, "model_calls": m.calls, "model_duration_ms": m.latency, "billing": "ChatGPT allowance; no API token price applied"})
				continue
			}
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
			ai.RecordAgentUsage(caller, m.model, m.input, m.output, m.calls, ai.UsageTiming{Account: account, RunID: s.RunID, ModelMS: m.latency})
		}
	}
}

// noUsageReported keeps that warning to once per process rather than once per
// question.
var noUsageReported sync.Once

// modelSpend is one model's share of one run.
type modelSpend struct {
	model   string
	input   int
	output  int
	calls   int
	latency int64
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
		m.latency += e.LatencyMS
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
