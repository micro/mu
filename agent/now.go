package agent

// What is already true, in front of the question.
//
// # The shape of the latency
//
// A question costs two model round trips before a word reaches the screen. The
// first decides which of thirty-odd tools to call and is invisible — nothing
// can be shown until it has decided. The tools themselves are cheap: they are
// in-process, over a memory transport, single-digit microseconds. Then a second
// call composes the answer, and only that one streams.
//
// So for "what are the headlines" the wait is almost entirely two model calls
// spent deciding to fetch, and then reading, something this instance already
// had in memory and had already rendered on the home page.
//
// # What Google is doing, and it is not a cleverer agent
//
// A search summary is fast because the retrieval already happened. Searching is
// the product, so by the time a model is invoked the results are in the prompt:
// one call, small model, streaming from the first token. It never asks itself
// whether to search.
//
// This is that. The services that already hold something say so — see
// service.Spec.Now — and it goes in the prompt ahead of the question. A
// question answered from it is one model call rather than three, and the tools
// are still there for everything else.
//
// # Why not keyword matching
//
// The obvious version is to match "bitcoin" and return the price. That is a
// search engine with an answer box, and it breaks on the first question that
// contains the word without being about it — "should I buy bitcoin", "why is
// bitcoin down", "remind me to sell bitcoin on Friday". Nothing here matches
// anything: the model still decides what the question means and what to do
// about it. It simply does not have to spend a round trip fetching what it was
// already holding.
//
// A keyword router lived here once, for choosing an agent rather than for this,
// and it is gone. See Routed.
//
// # What it costs
//
// Tokens on every question, including the ones it does not help. That is the
// whole reason each Now is a summary rather than a payload — eight headlines
// under their topics, five movers — and the reason for the two rules below.
//
// Only services this agent could have called anyway. An agent scoped to shell
// and apps gets nothing, because it could not have fetched the news either; the
// Code agent's prompt does not carry the markets. And a cap, so a service that
// grows a verbose Now cannot quietly put a page into every prompt on the
// instance.

import (
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/snapshot"
)

// nowBudget is the most this block may take, in characters.
//
// Roughly a thousand tokens. Past that it is no longer context, it is a page —
// and it would be paid for on every question, including the ones about the
// weather in a fortnight. A service whose Now overruns is dropped rather than
// truncated, because half a headline list read as a complete one is worse than
// not having it: the model would answer "that is all the news there is".
const nowBudget = 4000

// nowContext is the block of what the services in scope already know.
//
// Empty when nothing is in scope or nothing has anything to say, which is the
// ordinary case for a scoped agent and for a bare instance whose feeds have not
// run yet.
func nowContext(services []string) string {
	return nowContextFrom(services, service.Nows())
}

// nowContextFrom is nowContext over a given set of declarations.
//
// Split out so the rules here — what is in scope, what fits, what the model is
// told about it — can be tested without registering a service, which a test
// binary cannot do without a hook in production code that exists for the test.
func nowContextFrom(services []string, specs []service.Spec) string {
	if len(services) == 0 {
		return ""
	}
	inScope := map[string]bool{}
	for _, s := range services {
		inScope[strings.ToLower(strings.TrimSpace(s))] = true
	}

	// In service.Nows order, which is by name, so the same instance builds the
	// same prompt twice — a block whose sections move about is read differently
	// for no reason and defeats prompt caching on every provider that has it.
	var parts []string
	spent := 0
	for _, spec := range specs {
		if !inScope[spec.Name] {
			continue
		}
		text := strings.TrimSpace(published(spec))
		if text == "" {
			continue
		}
		if spent+len(text) > nowBudget {
			// Named, once, because the alternative is a service silently
			// dropping out of every prompt and nobody able to say when it
			// started.
			overBudget.Do(func() {
				app.Log("agent", "%s's Now is too long for the prompt budget (%d of %d "+
					"characters already spent) — it is being left out",
					spec.Name, spent, nowBudget)
			})
			continue
		}
		spent += len(text)
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return ""
	}
	// The instruction is the half that decides whether any of this helps.
	//
	// Without it the model reads a block of headlines and calls the news tool
	// anyway — the round trip is still spent and the tokens are wasted on top.
	// It has to say three things: that this is current, that answering from it
	// is allowed, and that the tools are still there when it is not enough.
	return "What you already know, as of this moment. It is current and it is " +
		"yours — answer from it directly when it covers the question, without " +
		"calling a tool for what is already written here. Call the tools when " +
		"you need more than this, or anything not in it.\n\n" +
		strings.Join(parts, "\n\n")
}

// published is what a service last put on the read plane, or — before it has
// put anything there — what it says right now.
//
// The plane is the point. A service publishes when its data changes, the same
// way it publishes its card, and every reader holds a mirror fed by the broker:
// so assembling this block is a handful of map reads rather than a call into
// each service, nothing a service does can sit on the answer path, and once the
// registry is networked the reader does not have to be in the same process as
// the producer. See internal/snapshot.
//
// The fallback is for the cold start and nothing else. A service publishes on
// its own refresh, so between this process starting and that first refresh the
// mirror is empty — and a question asked in that window should get the context
// rather than a principle. Calling Now here is safe by its own contract, which
// is that it reads memory; publishing the answer means the next reader takes
// the mirror.
func published(spec service.Spec) string {
	if v := snapshot.Value(spec.Name, "now"); v != "" {
		return v
	}
	if spec.Now == nil {
		return ""
	}
	text := spec.Now()
	snapshot.Channel(spec.Name, "now").Publish(text)
	return text
}

// overBudget keeps the warning to once per process rather than once per
// question.
var overBudget sync.Once
