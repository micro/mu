package agents

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
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "create":
			a, secret, err := Create(owner, r.FormValue("name"), r.FormValue("kind"),
				r.FormValue("prompt"), r.Form["services"])
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
			_ = Remove(owner, r.FormValue("id"))
			http.Redirect(w, r, "/agents?removed=1", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/agents", http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, List(owner))
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
	if secret := r.URL.Query().Get("secret"); secret != "" {
		b.WriteString(secretPanel(secret, Get(owner, r.URL.Query().Get("created")), app.BaseURL(r)))
	}

	agents := List(owner)
	if len(agents) == 0 {
		b.WriteString(`<p style="color:#888;font-size:14px">No agents yet. An agent is a name, a scope, ` +
			`and a token — so what you hand a credential to reaches the services you chose and no others.</p>`)
	} else {
		b.WriteString(`<h3 style="font-size:15px;margin:0 0 10px">Your agents</h3>`)
		b.WriteString(`<div style="display:flex;flex-direction:column;gap:8px;margin:0 0 24px">`)
		for _, a := range agents {
			b.WriteString(agentRow(a, csrf, app.BaseURL(r)))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(createForm(csrf))
	b.WriteString(agentsCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Agents", "The agents that act for you, and what each one may reach", b.String(), r)))
}

func urlSafe(s string) string { return strings.ReplaceAll(html.EscapeString(s), " ", "%20") }

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

	used := "never used"
	if !a.LastUsed.IsZero() {
		used = "last used " + a.LastUsed.Local().Format("2 Jan 15:04")
	}

	kind := "external"
	if a.Kind == Hosted {
		kind = "hosted here"
	}

	return fmt.Sprintf(`<div class="agent-row">
  <div style="flex:1;min-width:0">
    <div style="font-weight:600;font-size:14px">%s <span class="agent-kind">%s</span></div>
    <div class="%s">%s</div>
    <div class="agent-meta">%s · <code>%s</code></div>
  </div>
  <form method="POST" action="/agents" style="margin:0" onsubmit="return confirm('Remove this agent and revoke its token?')">
    <input type="hidden" name="_csrf" value="%s">
    <input type="hidden" name="action" value="delete">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="agent-remove">Remove</button>
  </form>
</div>`,
		html.EscapeString(a.Name), html.EscapeString(kind),
		cls, html.EscapeString(scope),
		html.EscapeString(used), html.EscapeString(a.Endpoint(base)),
		html.EscapeString(csrf), html.EscapeString(a.ID))
}

// createForm leads with the scope, because the scope is the decision.
func createForm(csrf string) string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h4 style="margin:0 0 4px;font-size:14px">New agent</h4>`)
	b.WriteString(`<p class="text-sm text-muted" style="margin:0 0 12px">It gets its own token. ` +
		`Pick what it may reach — it will be refused everything else, even though it is your account behind it.</p>`)
	b.WriteString(`<form method="POST" action="/agents">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">`)
	b.WriteString(`<input type="hidden" name="action" value="create">`)
	b.WriteString(`<input name="name" placeholder="What is it called? e.g. Morning briefer" required maxlength="60" class="agent-input">`)

	b.WriteString(`<div class="agent-kinds">`)
	b.WriteString(`<label><input type="radio" name="kind" value="external" checked> Runs elsewhere — Claude, Cursor, your own program, calling in with its token</label>`)
	b.WriteString(`<label><input type="radio" name="kind" value="hosted"> Runs here — give it standing instructions and this instance executes them</label>`)
	b.WriteString(`</div>`)

	b.WriteString(`<input name="prompt" placeholder="What is it for? (optional)" maxlength="500" class="agent-input">`)

	b.WriteString(`<div class="agent-scope-pick"><strong>What may it reach?</strong>`)
	b.WriteString(`<p class="text-sm text-muted" style="margin:2px 0 8px">Choose nothing and it reaches everything you can — which is what a plain token does, and rarely what you meant.</p>`)
	b.WriteString(`<div class="agent-services">`)
	for _, s := range scopeChoices() {
		b.WriteString(`<label><input type="checkbox" name="services" value="` +
			html.EscapeString(s.Name) + `"> ` + html.EscapeString(s.NavLabel()) + `</label>`)
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
.agent-remove{background:none;border:0;color:#bbb;font-size:13px;cursor:pointer}
.agent-remove:hover{color:#b00}
.agent-secret{background:#f5f5f5;padding:10px 12px;font-size:12px;overflow-x:auto;border-radius:6px;margin:0;word-break:break-all}
.agent-input{display:block;width:100%;padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit;margin:0 0 10px}
.agent-kinds{display:flex;flex-direction:column;gap:6px;margin:0 0 12px}
.agent-kinds label{font-size:13px;color:#444;display:flex;gap:8px;align-items:flex-start}
.agent-scope-pick{border-top:1px solid #eee;padding-top:12px;margin:0 0 14px}
.agent-services{display:grid;grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:4px 12px}
.agent-services label{font-size:13px;color:#444;display:flex;align-items:center;gap:6px}
@media only screen and (max-width:600px){.agent-services{grid-template-columns:1fr 1fr}}
</style>`
