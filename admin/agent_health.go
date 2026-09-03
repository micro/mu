package admin

// How the agent is behaving, as against what it costs.
//
// The spend card next door answers "what have I paid", which the instance could
// always answer. This answers "is it working", which it could not: go-micro
// records a line per model call and per tool call — status, error, latency,
// retries — agent/cost.go loaded every run's timeline to price it, and read the
// tokens off it and dropped the rest. See agent/outcome.go.
//
// Two lists carry the weight. What gets called is the evidence for or against
// narrowing the tool catalogue, which is offered a hundred and nineteen tools
// wide to every signed-in question and had never been checked against what the
// model actually reaches for. What goes wrong is the list that turns "make it
// reliable" into work.
//
// On the spend tab rather than a page of its own. A run's cost and a run's
// outcome are two readings of the same event, and an operator looking at one
// wants the other in the same glance — a separate page would be two places to
// go to ask one question.

import (
	"fmt"
	"html"
	"strings"

	"mu/agent"
)

func agentHealthCard() string {
	h := agent.Summary()

	var sb strings.Builder
	sb.WriteString(`<h3>Agent runs</h3>`)
	if h.Runs == 0 {
		// Said plainly. Nothing recorded is a fact about this process, not a
		// fact about the agent — the record is in memory and starts empty after
		// a restart, so an empty card on a box that has been up ten seconds
		// means nothing is wrong.
		sb.WriteString(`<p class="text-sm text-muted">No runs since this process started. ` +
			`The record is the last 100 runs, held in memory.</p>`)
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf(`<p><strong>%d runs</strong> — %d did not finish, `+
		`%d had a model call retried, %d tool calls came back an error.</p>`,
		h.Runs, h.Failed, h.Retried, h.ToolErrors))
	sb.WriteString(fmt.Sprintf(`<p class="text-sm">Steps per run: %d median, %d at the longest. `+
		`The loop is capped at 40.</p>`, h.MedianStep, h.MaxStep))

	if len(h.TopTools) > 0 {
		sb.WriteString(`<h3>What it calls</h3>`)
		sb.WriteString(`<div class="scroll-x"><table class="ai-usage-table"><thead><tr>` +
			`<th>Tool</th><th>Calls</th></tr></thead><tbody>`)
		for _, c := range h.TopTools {
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`,
				html.EscapeString(c.Name), c.N))
		}
		sb.WriteString(`</tbody></table></div>`)
	}

	if len(h.TopErrors) > 0 {
		sb.WriteString(`<h3>What goes wrong</h3>`)
		sb.WriteString(`<div class="scroll-x"><table class="ai-usage-table"><thead><tr>` +
			`<th>Error</th><th>Runs</th></tr></thead><tbody>`)
		for _, c := range h.TopErrors {
			// A provider's message, so it is escaped: it is text from
			// somewhere else arriving on an operator's page.
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`,
				html.EscapeString(c.Name), c.N))
		}
		sb.WriteString(`</tbody></table></div>`)
	}
	return sb.String()
}
