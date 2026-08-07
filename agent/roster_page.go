package agent

// The agents page: what acts for you, and what each one may touch.
//
// The create form leads with the scope rather than burying it in an "advanced"
// fold, because the scope is the decision. Everything else on the form is a
// label. A page that asks for a name and hands back a credential with your whole
// account behind it is the thing this replaces.
//
// Nothing here is checked twice: the scope chosen here is written into the
// token's permissions, and the MCP boundary enforces it against every call. This
// page cannot grant more than it shows, because it is not the thing doing the
// granting.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Handler serves /agents.
func RosterHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "create":
			// The kind decides whether a credential is issued, because the kind
			// is a statement about where the agent runs.
			//
			// "Runs elsewhere" is Claude or your own program calling in, and a
			// token is the only way it can: withholding one would ship
			// something that cannot work. "Runs here" is standing instructions
			// this instance executes — nothing outside needs to call it, so
			// minting a credential and printing an MCP endpoint answers a
			// question nobody asked and leaves a live secret lying around.
			//
			// Both options used to get a token anyway, which made the choice
			// cosmetic and made "Issue token" on the row unreachable. Now it is
			// there for the case it was written for: a local agent you later
			// decide to call from outside.
			kind := r.FormValue("kind")
			a, secret, err := CreateAgent(owner, r.FormValue("name"), kind,
				r.FormValue("prompt"), "", r.Form["services"], issuesToken(kind))
			if err != nil {
				http.Redirect(w, r, "/agents?error="+urlSafe(err.Error()), http.StatusSeeOther)
				return
			}
			// The secret rides back in the URL once. It is never stored in
			// readable form and cannot be shown again, which is the whole
			// reason this page has a "copy it now" state at all.
			http.Redirect(w, r, "/agents?created="+a.ID+"&secret="+urlSafe(secret), http.StatusSeeOther)
			return
		case "delete":
			_ = RemoveAgent(owner, r.FormValue("id"))
			http.Redirect(w, r, "/agents?removed=1", http.StatusSeeOther)
			return
		case "token":
			secret, err := IssueToken(owner, r.FormValue("id"))
			if err != nil {
				http.Redirect(w, r, "/agents?error="+urlSafe(err.Error()), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/agents?created="+r.FormValue("id")+"&secret="+urlSafe(secret), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/agents", http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, Agents(owner))
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(`<div style="max-width:720px">`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<p class="text-error">` + html.EscapeString(msg) + `</p>`)
	}
	if r.URL.Query().Get("removed") != "" {
		b.WriteString(`<p class="text-sm" style="color:#666">Agent removed and its token revoked.</p>`)
	}
	created := r.URL.Query().Get("created")
	if secret := r.URL.Query().Get("secret"); secret != "" {
		b.WriteString(secretPanel(secret, AgentFor(owner, created), app.BaseURL(r)))
	} else if created != "" {
		// An agent that runs here has no secret to show, so say what happened
		// and what it is now for — otherwise the page just silently grows a row.
		name := "It"
		if a := AgentFor(owner, created); a != nil {
			name = html.EscapeString(a.Name)
		}
		b.WriteString(`<p class="text-sm" style="color:#666">` + name + ` is ready. Open its name below ` +
			`to talk to it here — it has no token, so nothing outside this instance can call it.</p>`)
	}

	roster := Agents(owner)
	if len(roster) == 0 {
		b.WriteString(`<p style="color:#888;font-size:14px">No agents yet. An agent is a name, a scope, ` +
			`and a token — so what you hand a credential to reaches the services you chose and no others.</p>`)
	} else {
		b.WriteString(`<h3 style="font-size:15px;margin:0 0 10px">Your agents</h3>`)
		b.WriteString(`<div style="display:flex;flex-direction:column;gap:8px;margin:0 0 24px">`)
		for _, a := range roster {
			b.WriteString(agentRow(a, csrf, app.BaseURL(r)))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(createForm(csrf))
	b.WriteString(agentsCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Agents", "The agents that act for you, and what each one may reach", b.String(), r)))
}

func urlSafe(s string) string { return strings.ReplaceAll(html.EscapeString(s), " ", "%20") }

// issuesToken says whether creating an agent of this kind should mint a
// credential.
//
// A token is what lets something outside this instance call in, so the answer
// is exactly "does this agent run outside". One that runs here is reached
// through the agent page and needs nothing; one that runs elsewhere cannot work
// without a token, so withholding it would ship something broken.
//
// Anything unrecognised is treated as external, because that is what the form
// defaults to and an agent that cannot be called is the more confusing failure.
func issuesToken(kind string) bool { return kind != Hosted }

// secretPanel shows the token once. There is no second chance by design: it is
// stored hashed, so this page could not show it again if it wanted to.
func secretPanel(secret string, a *Agent, base string) string {
	name, endpoint := "your agent", base+"/mcp"
	if a != nil {
		name, endpoint = a.Name, a.Endpoint(base)
	}
	return `<div class="card" style="border-color:#0a7d33;margin:0 0 20px">` +
		`<h4 style="margin:0 0 6px;font-size:14px">` + html.EscapeString(name) + ` is ready</h4>` +
		`<p class="text-sm text-muted" style="margin:0 0 8px">Copy this token now — it is stored hashed and cannot be shown again.</p>` +
		`<pre class="agent-secret">` + html.EscapeString(secret) + `</pre>` +
		`<p class="text-sm text-muted" style="margin:8px 0 0">Point it at <code>` + html.EscapeString(endpoint) + `</code></p>` +
		`</div>`
}

// agentRow shows an agent the way it is actually used.
//
// It used to show every agent the same: an MCP endpoint, "never used", and an
// "Issue token" button with no hint whether one was needed. For an agent you
// only talk to here that is three kinds of wrong at once — the endpoint cannot
// be called without a token, "never used" describes a token that does not
// exist, and the button reads like a required step. The first question anybody
// asked was "do I need to issue a token?", which is the page's fault.
//
// So it branches on whether there is a credential, not on the declared kind:
// what matters is whether this thing can be called from outside, and a token is
// exactly what decides that.
func agentRow(a *Agent, csrf, base string) string {
	scope := "everything you can reach"
	cls := "agent-scope wide"
	if !a.Unscoped() {
		labels := make([]string, 0, len(a.Services))
		for _, s := range a.Services {
			labels = append(labels, service.Label(s))
		}
		scope = strings.Join(labels, ", ")
		cls = "agent-scope"
	}

	// What this agent is, in terms of what you can do with it.
	meta, action := "", ""
	if a.TokenID == "" {
		// The name above is the link now, so this says what is true rather than
		// repeating where to click.
		meta = `Open its name to talk to it here. No token, so nothing outside this instance can call it.`
		action = fmt.Sprintf(`<form method="POST" action="/agents" style="margin:0">
    <input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="token">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="agent-act" title="Only needed to call it from outside">Issue token</button>
  </form>`, html.EscapeString(csrf), html.EscapeString(a.ID))
	} else {
		used := "not called yet"
		if !a.LastUsed.IsZero() {
			used = "last called " + a.LastUsed.Local().Format("2 Jan 15:04")
		}
		meta = html.EscapeString(used) + ` · <code>` + html.EscapeString(a.Endpoint(base)) + `</code>`
	}

	return fmt.Sprintf(`<div class="agent-row">
  <div style="flex:1;min-width:0">
    <a class="agent-name" href="/agent?agent=%s">%s</a>
    <div class="%s">%s</div>
    <div class="agent-meta">%s</div>
  </div>
  %s
  <form method="POST" action="/agents" style="margin:0" onsubmit="return confirm('Remove this agent?')">
    <input type="hidden" name="_csrf" value="%s">
    <input type="hidden" name="action" value="delete">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="agent-remove">Remove</button>
  </form>
</div>`,
		html.EscapeString(a.ID), html.EscapeString(a.Name),
		cls, html.EscapeString(scope),
		meta, action,
		html.EscapeString(csrf), html.EscapeString(a.ID))
}

// createForm leads with the scope, because the scope is the decision.
func createForm(csrf string) string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h4 style="margin:0 0 4px;font-size:14px">New agent</h4>`)
	b.WriteString(`<p class="text-sm text-muted" style="margin:0 0 12px">One that runs elsewhere gets its own ` +
		`token; one that runs here does not need one. Either way, pick what it may reach — it will be refused ` +
		`everything else, even though it is your account behind it.</p>`)
	b.WriteString(`<form method="POST" action="/agents">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">`)
	b.WriteString(`<input type="hidden" name="action" value="create">`)
	b.WriteString(`<input name="name" placeholder="What is it called? e.g. Morning briefer" required maxlength="60" class="agent-input">`)

	// A segmented control rather than two radios. An inline <input> next to
	// text never lines up: the box sits on the text baseline, its height is the
	// browser's rather than the line's, and every fix is a different magic
	// number per browser. Hiding the input and styling its label removes the
	// alignment problem instead of tuning it — there is nothing inline left to
	// align, and the whole row is the hit target.
	b.WriteString(`<div class="pick-row">`)
	b.WriteString(`<label class="pick"><input type="radio" name="kind" value="external" checked>` +
		`<span><strong>Runs elsewhere</strong>Claude, Cursor, or your own program, calling in with its token</span></label>`)
	b.WriteString(`<label class="pick"><input type="radio" name="kind" value="hosted">` +
		`<span><strong>Runs here</strong>Give it standing instructions and this instance executes them</span></label>`)
	b.WriteString(`</div>`)

	b.WriteString(`<input name="prompt" placeholder="What is it for? (optional)" maxlength="500" class="agent-input">`)

	b.WriteString(`<div class="agent-scope-pick"><strong>What may it reach?</strong>`)
	b.WriteString(`<p class="text-sm text-muted" style="margin:2px 0 8px">Choose nothing and it reaches everything you can — which is what a plain token does, and rarely what you meant.</p>`)
	b.WriteString(`<div class="agent-services">`)
	for _, s := range scopeChoices() {
		b.WriteString(`<label class="chip"><input type="checkbox" name="services" value="` +
			html.EscapeString(s.Name) + `"><span>` + html.EscapeString(s.NavLabel()) + `</span></label>`)
	}
	b.WriteString(`</div></div>`)

	b.WriteString(`<button type="submit">Create agent</button>`)
	b.WriteString(`</form></div>`)
	return b.String()
}

// scopeChoices is every service that can be named in a scope, by label.
func scopeChoices() []service.Spec {
	specs := service.Specs()
	sort.Slice(specs, func(i, j int) bool { return specs[i].NavLabel() < specs[j].NavLabel() })
	return specs
}

const agentsCSS = `<style>
.agent-row{display:flex;align-items:center;gap:12px;border:1px solid #eee;border-radius:8px;padding:10px 14px}
.agent-kind{font-size:11px;color:#999;font-weight:400;margin-left:4px}
.agent-scope{font-size:13px;color:#0a7d33}
.agent-scope.wide{color:#a86400}
.agent-meta{font-size:12px;color:#999;margin-top:2px;overflow:hidden;text-overflow:ellipsis}
.agent-meta code{font-size:11px}
/* The agent's name is the way into it, so it looks like body text until you
   are over it rather than like one more small grey control. */
.agent-name{display:inline-block;font-weight:600;font-size:14px;color:var(--text-primary,#111);text-decoration:none}
.agent-name:hover{text-decoration:underline}
/* Two buttons that did the same thing to the eye and different things to the
   account. Both were #bbb — barely visible — and both took the same red hover,
   which said "destructive" about issuing a token. Now issuing reads as an
   ordinary action and only Remove goes red, which is the one that is. */
.agent-act{background:none;border:0;color:#666;font-size:13px;cursor:pointer;padding:0}
.agent-act:hover{color:#111;text-decoration:underline}
.agent-remove{background:none;border:0;color:#999;font-size:13px;cursor:pointer;padding:0}
.agent-remove:hover{color:#b00;text-decoration:underline}
.agent-secret{background:#f5f5f5;padding:10px 12px;font-size:12px;overflow-x:auto;border-radius:6px;margin:0;word-break:break-all}
.agent-input{display:block;width:100%;padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit;margin:0 0 10px}
/* The input is hidden and its label carries the state. Nothing is inline, so
   nothing can be misaligned; :checked styles the sibling span. */
.pick input,.chip input{position:absolute;opacity:0;width:0;height:0}
.pick input:focus-visible+span,.chip input:focus-visible+span{outline:2px solid #111;outline-offset:2px}

.pick-row{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin:0 0 14px}
.pick span{display:block;height:100%;border:1px solid #ddd;border-radius:8px;padding:10px 12px;
  cursor:pointer;font-size:12px;color:#666;line-height:1.4}
.pick strong{display:block;font-size:13px;color:#111;margin:0 0 2px}
.pick span:hover{border-color:#bbb}
.pick input:checked+span{border-color:#111;background:#fafafa}
.pick input:checked+span strong::after{content:" ✓";color:#0a7d33}

.agent-scope-pick{border-top:1px solid #eee;padding-top:12px;margin:0 0 14px}
.agent-services{display:flex;flex-wrap:wrap;gap:6px}
.chip span{display:block;border:1px solid #ddd;border-radius:999px;padding:6px 12px;
  cursor:pointer;font-size:13px;color:#444;white-space:nowrap}
.chip span:hover{border-color:#bbb}
.chip input:checked+span{background:#111;border-color:#111;color:#fff}
@media only screen and (max-width:600px){.pick-row{grid-template-columns:1fr}}
</style>`
