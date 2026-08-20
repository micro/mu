package agent

// What an agent has done, where it is worth showing.
//
// This was a page: /agent/runs, a tab beside the chat, listing every workflow
// record — prompt, agent, tools, whether it finished. It was built because
// nothing showed that a run had happened, and it answered that by adding a
// fourth name to a surface that already had three. Chat, Threads, Runs,
// Connect, for what is one thing: you, an agent, and what you said to each
// other.
//
// The information was worth keeping and the page was not. What tools an answer
// came from, and the error when it failed, now sit next to the answer in the
// conversation — see runTools in conversation.go — which is where somebody
// looking at an odd answer actually is. What remains here is the row, which the
// agent builder still uses to show what one agent has been doing.

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"mu/internal/app"
)

// runRow is one run. csrf empty means no delete control, for anywhere this is
// rendered as a summary rather than as the page you came to manage runs on.
func runRow(f *Flow, csrf string) string {
	status, cls := "done", "run-ok"
	switch f.Status {
	case "running":
		status, cls = "running", "run-going"
	case "error":
		status, cls = "failed", "run-bad"
	}

	// What it ran, deduplicated: a tool called three times is one thing it did,
	// and three identical chips read as a stutter rather than as information.
	var tools []string
	seen := map[string]bool{}
	for _, s := range f.Steps {
		if s.Tool == "" || seen[s.Tool] {
			continue
		}
		seen[s.Tool] = true
		tools = append(tools, s.Tool)
	}
	ran := `<span class="run-none">no tools</span>`
	if len(tools) > 0 {
		var chips strings.Builder
		for _, t := range tools {
			chips.WriteString(app.Pill(t))
		}
		ran = chips.String()
	}

	as := "Micro"
	if f.Agent != "" {
		// The name if it still exists; agents get deleted and their runs do not.
		//
		// A run can also name one of this instance's own agents — somebody wrote
		// to agent+markets@ — which is nobody's roster entry and would otherwise
		// read as removed. Those do not get deleted, so the name is always there.
		if a := For(f.AccountID, f.Agent); a != nil {
			as = a.Name
		} else if name := platformName(f.Agent); name != "" {
			as = name
		} else {
			as = "a removed agent"
		}
	}

	prompt := f.Prompt
	if len(prompt) > 140 {
		prompt = strings.TrimSpace(prompt[:140]) + "…"
	}

	del := ""
	if csrf != "" {
		del = fmt.Sprintf(`<form method="POST" action="/agents" style="margin:0" onsubmit="return confirm('Delete this run?')">
    <input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="delete-run">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="run-del">Delete</button></form>`,
			html.EscapeString(csrf), html.EscapeString(f.ID))
	}

	failed := ""
	if f.Status == "error" && f.Error != "" {
		failed = `<div class="run-err">` + html.EscapeString(f.Error) + `</div>`
	}

	// Where it came from, but only when it came from somewhere. Every run
	// started at the page would otherwise carry a chip saying "page", which is
	// noise on the common case — and the point of the chip is that a run
	// somebody else set off should not look like one you asked for.
	came := ""
	if f.Source != "" {
		what := f.Trigger
		if what == "" {
			what = f.Source
		}
		came = `<span class="run-from">` + html.EscapeString(what) + `</span>`
	}

	return fmt.Sprintf(`<div class="run-row">
  <div style="flex:1;min-width:0">
    <a class="run-prompt" href="/agent?session=%s">%s</a>
    <div class="run-meta"><span class="%s">%s</span> · as %s · %s %s</div>
    <div class="run-tools">%s</div>
    %s
  </div>
  %s
</div>`,
		html.EscapeString(f.ID), html.EscapeString(prompt),
		cls, status, html.EscapeString(as), html.EscapeString(app.TimeAgo(f.CreatedAt)), came,
		ran, failed, del)
}

const runsCSS = `<style>
.runs{display:flex;flex-direction:column;gap:8px}
.run-row{display:flex;align-items:flex-start;gap:12px;border:1px solid #eee;border-radius:8px;padding:10px 14px}
.run-prompt{display:inline-block;font-weight:600;font-size:14px;color:var(--text-primary,#111);text-decoration:none}
.run-prompt:hover{text-decoration:underline}
.run-meta{font-size:12px;color:#999;margin-top:3px}
.run-tools{display:flex;flex-wrap:wrap;gap:4px;margin-top:6px}
.run-from{border:1px solid #e6e0cf;background:#fdfbf3;border-radius:6px;padding:1px 8px;font-size:11px;color:#8a7a4a;white-space:nowrap}
.run-none{font-size:11px;color:#bbb}
.run-err{font-size:12px;color:#b00;margin-top:5px}
.run-ok{color:#0a7d33}
.run-going{color:#a86400}
.run-bad{color:#b00}
.run-del{background:none;border:0;color:#999;font-size:13px;cursor:pointer;padding:0}
.run-del:hover{color:#b00;text-decoration:underline}
.lens-lead{color:#666;font-size:14px;margin:0 0 18px;max-width:640px}
</style>`

// ── Asking a named agent from anywhere ──────────────────────────
//
// Agents were reachable only through this instance's own web chat. Every other
// caller — MCP, Discord, Telegram, the CLI, a cron job — got the default
// assistant however many you had built, because the only place that resolved a
// named agent was the SSE handler. An agent you cannot invoke from outside is a
// preset on a settings page.

// AskAs turns a name or id into the options a query should run with. An empty
// ref means the default agent, which is not an error — most callers want it.
func AskAs(accountID, ref string) (QueryOpts, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return QueryOpts{}, nil
	}
	a := findAgent(accountID, ref)
	if a == nil {
		return QueryOpts{}, fmt.Errorf("no agent called %q — agent_list names yours", ref)
	}
	m := a.AsMicro()
	return QueryOpts{System: m.SystemPrompt, Tools: m.Tools}, nil
}

// findAgent resolves by id first, then by name, case-insensitively. By name
// because somebody writing a call by hand knows what they named it and does not
// know its uuid; by id first because an id is unambiguous and a name need not be.
func findAgent(accountID, ref string) *Agent {
	if a := For(accountID, ref); a != nil {
		return a
	}
	want := strings.ToLower(ref)
	for _, a := range Agents(accountID) {
		if strings.ToLower(a.Name) == want {
			return a
		}
	}
	// A published agent somebody else owns, by id. Same rules as the web path:
	// it runs here, on this account, with this account's scope.
	if a := RunPublic(accountID, ref); a != nil {
		return a
	}
	return nil
}

// ListForCaller names an account's agents as JSON, for agent_list.
func ListForCaller(accountID string) (string, error) {
	type lite struct {
		Name  string   `json:"name"`
		About string   `json:"about,omitempty"`
		Tools []string `json:"tools,omitempty"`
		Email string   `json:"email,omitempty"`
	}
	out := []lite{{Name: "Micro", About: "The default agent. Every tool you can reach."}}
	for _, a := range Agents(accountID) {
		out = append(out, lite{
			Name:  a.Name,
			About: firstLine(a.Description, a.Prompt),
			Tools: a.Services,
			Email: a.Address(),
		})
	}
	b, err := json.Marshal(map[string]any{"agents": out})
	return string(b), err
}

// agentRunsSummary is what one agent has actually done, for the page where you
// scope it.
//
// An agent is a name, a standing instruction and a set of tools it may reach —
// a scope. The page where you set that scope said nothing about whether any of
// it worked: no runs, no errors, no sign the tools you picked were the tools it
// needed. The evidence existed, on /runs, mixed in with every other agent's and
// with no way to ask for one agent's.
//
// The last few here, and a link to the rest filtered to this agent.
func agentRunsSummary(accountID, agentID string) string {
	var mine []*Flow
	for _, f := range ListFlows(accountID) {
		if f.Agent == agentID {
			mine = append(mine, f)
		}
	}

	var b strings.Builder
	b.WriteString(`<label class="b-label">What it has done</label>`)
	if len(mine) == 0 {
		b.WriteString(`<p class="b-state">Nothing yet. Ask it something and its runs — the tools ` +
			`it called and whether they worked — show up here.</p>` + runsCSS)
		return b.String()
	}

	shown := mine
	if len(shown) > 3 {
		shown = shown[:3]
	}
	b.WriteString(`<div class="runs">`)
	for _, f := range shown {
		b.WriteString(runRow(f, ""))
	}
	b.WriteString(`</div>`)
	if len(mine) > len(shown) {
		b.WriteString(fmt.Sprintf(`<p class="b-state">and %d more</p>`, len(mine)-len(shown)))
	}
	return b.String() + runsCSS
}
