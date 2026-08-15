package agent

// What your agents actually did, and whether it worked.
//
// Every question already produced a Flow: the prompt, the agent it was asked
// as, each tool it ran, the answer, and the error if there was one. Persisted
// per account, evicted by age. ListFlows existed. Nothing served it — flows
// were read only to restore a chat, so the record of every run an agent ever
// made was written and never shown.
//
// That is the difference between a thing you delegate to and a thing you watch
// do the work. A question you asked and watched answer needs no receipt; a task
// that ran at 6am, or an agent somebody else is driving over MCP, is invisible
// without one. docs/PRODUCT.md called this the missing piece and it was right:
// the product never told you when something worked.
//
// The list is deliberately thin — when, what was asked, who answered, what it
// ran, and whether it finished. A run opens as what it is: a turn in a
// conversation, in the chat, with that agent selected and its other
// conversations beside it.
//
// It links at /agent?session= directly. /agent/flow/<id> still exists and
// redirects there — flows became conversations — so linking at the old path
// meant every click took two requests to land somewhere the first one already
// knew about.

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// RunsHandler serves /agent/runs.
func RunsHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost && r.FormValue("action") == "delete" {
		_ = deleteFlow(owner, r.FormValue("id"))
		http.Redirect(w, r, "/agent/runs", http.StatusSeeOther)
		return
	}

	runs := ListFlows(owner)

	// One agent's runs. The builder links here so "what has this agent done"
	// has an answer past the three it shows inline, and an agent you drive over
	// MCP is the case with no other way to see it at all.
	only := strings.TrimSpace(r.URL.Query().Get("agent"))
	onlyName := ""
	if only != "" {
		var mine []*Flow
		for _, f := range runs {
			if f.Agent == only {
				mine = append(mine, f)
			}
		}
		runs = mine
		onlyName = only
		if a := For(owner, only); a != nil {
			onlyName = a.Name
		}
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"runs": runs})
		return
	}

	var b strings.Builder
	b.WriteString(`<div style="max-width:820px">`)
	b.WriteString(agentTabs("runs", only))
	b.WriteString(`<p class="lens-lead">A trace of every question your agents have answered: what was ` +
		`asked, which agent took it, which tools it called, and whether it finished. Runs started by ` +
		`a task, a schedule, or an agent calling in over MCP land here too — those are the ones ` +
		`nobody watched. Open one to see the answer and its sources. ` +
		`What it all cost is on ` + app.Link("Usage", "/usage") + `</p>`)

	if onlyName != "" {
		b.WriteString(`<p class="lens-lead" style="margin-top:-8px">Showing only <strong>` +
			html.EscapeString(onlyName) + `</strong>. ` + app.Link("Every agent", "/agent/runs") + `</p>`)
	}

	if len(runs) == 0 {
		b.WriteString(`<p style="color:#888;font-size:14px">Nothing yet. Ask an agent something and ` +
			`it will appear here.</p>`)
	} else {
		b.WriteString(`<div class="runs">`)
		for _, f := range runs {
			b.WriteString(runRow(f, auth.CSRFToken(r)))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>` + runsCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Runs", "What your agents did, and whether it worked", b.String(), r)))
}

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
			chips.WriteString(`<span class="run-tool">` + html.EscapeString(t) + `</span>`)
		}
		ran = chips.String()
	}

	as := "Micro"
	if f.Agent != "" {
		// The name if it still exists; agents get deleted and their runs do not.
		if a := For(f.AccountID, f.Agent); a != nil {
			as = a.Name
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
		del = fmt.Sprintf(`<form method="POST" action="/agent/runs" style="margin:0" onsubmit="return confirm('Delete this run?')">
    <input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="delete">
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
.run-tool{border:1px solid #eee;border-radius:999px;padding:2px 9px;font-size:11px;color:#666;white-space:nowrap}
.run-from{border:1px solid #e6e0cf;background:#fdfbf3;border-radius:999px;padding:1px 8px;font-size:11px;color:#8a7a4a;white-space:nowrap}
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
		b.WriteString(`<p class="b-state">` +
			app.Link(fmt.Sprintf("All %d runs by this agent", len(mine)),
				"/agent/runs?agent="+url.QueryEscape(agentID)) + `</p>`)
	}
	return b.String() + runsCSS
}

// agentTabs is the strip that switches between talking to an agent and seeing
// what it has done.
//
// Runs used to be /runs, a top-level page beside Home and Tools, which put the
// record of what your agents did at the same level as the things themselves. It
// is not a peer of the agent; it is the other half of one. So it lives under
// /agent and the two tab between each other, carrying the selected agent across
// so "chat to this one" and "what has this one done" are the same question
// asked twice.
func agentTabs(active, agentID string) string {
	q := ""
	if agentID != "" {
		q = "?" + url.Values{"id": {agentID}}.Encode()
	}
	runsQ := ""
	if agentID != "" {
		runsQ = "?" + url.Values{"agent": {agentID}}.Encode()
	}
	tab := func(label, href, key string) string {
		cls := "agent-tab"
		if key == active {
			cls += " on"
		}
		return `<a class="` + cls + `" href="` + href + `">` + label + `</a>`
	}
	out := `<div class="agent-tabs">` + tab("Chat", "/agent"+q, "chat")
	// Connect is only a tab when there is an agent to connect to. The default
	// assistant has no token, no scope and no address of its own.
	if agentID != "" {
		out += tab("Connect", "/agent/connect"+q, "connect")
	}
	return out + tab("Runs", "/agent/runs"+runsQ, "runs") + `</div>` + agentTabsCSS
}

const agentTabsCSS = `<style>
.agent-tabs{display:flex;gap:2px;margin:0 0 16px;border-bottom:1px solid var(--border-color,#e5e5e5)}
.agent-tab{padding:7px 14px;font-size:14px;color:var(--text-muted,#666);text-decoration:none;
  border-bottom:2px solid transparent;margin-bottom:-1px}
.agent-tab:hover{color:var(--text-primary,#111)}
.agent-tab.on{color:var(--text-primary,#111);font-weight:600;border-bottom-color:var(--text-primary,#111)}
</style>`
