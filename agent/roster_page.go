package agent

// The agents page: what acts for you, and what each one may touch.
//
// It lists and it revokes; it does not create. Making an agent happens in one
// place, the builder at /agent/new, because this page used to carry a second
// create form that asked different questions and issued a different result.
//
// Nothing here is checked twice: the scope chosen in the builder is written into
// the token's permissions, and the MCP boundary enforces it against every call.
// This page cannot grant more than it shows, because it is not the thing doing
// the granting.

import (
	"fmt"
	"html"
	"net/http"
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
	// The first line of the same three-page sentence: an agent acts for you,
	// tools are what it can call, services are what runs behind them.
	b.WriteString(`<p class="lens-lead">What acts for you. Give one a standing instruction and ` +
		`the services it may reach, then talk to it here or hand it a token and run it from ` +
		`Claude, Cursor or your own program.</p>`)

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

	EnsureTags(owner)
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

	// One way to make an agent. This page used to carry its own create form
	// alongside the builder at /agent/new, and the two disagreed: one asked for
	// services and issued a token, the other asked for tools and a system prompt
	// and issued nothing, and neither linked to the other. The builder took the
	// missing question ("where does it run?") so this could become a link.
	b.WriteString(app.ActionLink("/agent/new", "+ New agent"))
	// Close the column this page opened. Without it the footer was swallowed
	// into a 720px div and rendered halfway up the page: #content is a flex
	// child sized to hold the footer at the bottom, and an unclosed div puts
	// the footer inside the content instead of after it.
	b.WriteString(`</div>`)
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
	//
	// An agent that runs here is offered no token. It was offered one because
	// the button branched on whether a credential existed rather than on whether
	// one could be used, so "runs here" — the option whose whole meaning is that
	// nothing outside calls it — got a button inviting you to make that untrue.
	// Changing where an agent runs is editing it, and editing is /agent/new.
	meta, action := "", ""
	if a.TokenID == "" {
		// The name above is the link now, so this says what is true rather than
		// repeating where to click.
		meta = `Open its name to talk to it here. No token, so nothing outside this instance can call it.`
		if a.Kind != Hosted {
			action = fmt.Sprintf(`<form method="POST" action="/agents" style="margin:0">
    <input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="token">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="agent-act" title="It runs elsewhere, so it needs a credential to call in">Issue token</button>
  </form>`, html.EscapeString(csrf), html.EscapeString(a.ID))
		}
	} else {
		used := "not called yet"
		if !a.LastUsed.IsZero() {
			used = "last called " + a.LastUsed.Local().Format("2 Jan 15:04")
		}
		meta = html.EscapeString(used) + ` · <code>` + html.EscapeString(a.Endpoint(base)) + `</code>`
	}

	// The address is on the row because it is the thing you have to copy
	// somewhere else — into a form, a signup, a person's contacts. An agent that
	// can be written to is most of what makes it more than a preset, and it was
	// previously reachable only by knowing the plus-address convention.
	addr := ""
	if a.Address() != "" {
		addr = `<div class="agent-mail">Write to it at <code>` + html.EscapeString(a.Address()) + `</code></div>`
	}

	// Where it runs, on the row, in a word.
	//
	// It was only ever asked at creation and never shown again — not here, not
	// in the editor — so once an agent existed there was nowhere in the product
	// that told you whether it ran on this instance or somewhere else calling
	// in. That is the difference between a thing you talk to and a thing you
	// point Claude Desktop at, and it was inferable at best from whether a
	// token happened to be mentioned further down the row.
	kind := `<span class="agent-kind here">Runs here</span>`
	if a.Kind != Hosted {
		kind = `<span class="agent-kind away">Runs elsewhere</span>`
	}

	// The two things you come to this page to do to an agent that are not
	// talking to it: change it, and see what it did.
	//
	// Edit was missing entirely. The rail on /agent has had a pencil on every
	// row for a while, so the builder was reachable — from the chat, in a
	// 250px column, behind an icon — while the page whose whole job is managing
	// agents offered no way to change one. You could make an agent here, name
	// it, scope it, delete it, and not edit it.
	links := fmt.Sprintf(`<div class="agent-links">`+
		`<a href="/agent/connect?id=%s">Connect</a>`+
		`<a href="/agent/new?id=%s">Edit</a>`+
		`<a href="/agent/new?fork=%s">Fork</a>`+
		`<a href="/agent/runs?agent=%s">Runs &rarr;</a></div>`,
		html.EscapeString(a.ID), html.EscapeString(a.ID),
		html.EscapeString(a.ID), html.EscapeString(a.ID))

	// Where the name goes depends on what the agent is for. One that runs here
	// opens on talking to it; one that runs elsewhere opens on how to reach it,
	// because a chat box is the least useful half of that page and the endpoint,
	// the scope and the token were nowhere on it.
	open := "/agent?id=%s"
	if a.Kind != Hosted {
		open = "/agent/connect?id=%s"
	}

	return fmt.Sprintf(`<div class="agent-row">
  <div style="flex:1;min-width:0">
    <a class="agent-name" href="`+open+`">%s</a>`+kind+`
    <div class="%s">%s</div>
    <div class="agent-meta">%s</div>
    %s
    `+links+`
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
		meta, addr, action,
		html.EscapeString(csrf), html.EscapeString(a.ID))
}

const agentsCSS = `<style>
.lens-lead{color:#666;font-size:14px;margin:0 0 18px;max-width:640px}
.agent-row{display:flex;align-items:center;gap:12px;border:1px solid #eee;border-radius:8px;padding:10px 14px}
.agent-kind{display:inline-block;font-size:11px;font-weight:600;margin-left:8px;
  padding:1px 7px;border-radius:999px;vertical-align:middle}
.agent-kind.here{color:#0a7d33;background:#eaf6ee}
.agent-kind.away{color:#a86400;background:#fdf3e3}
.agent-links{display:flex;flex-wrap:wrap;gap:12px;margin-top:6px}
.agent-links a{font-size:12px;color:#666;text-decoration:none}
.agent-links a:hover{color:var(--text-primary,#111);text-decoration:underline}
.agent-scope{font-size:13px;color:#0a7d33}
.agent-scope.wide{color:#a86400}
.agent-meta{font-size:12px;color:#999;margin-top:2px;overflow:hidden;text-overflow:ellipsis}
.agent-meta code{font-size:11px}
.agent-mail{font-size:12px;color:#666;margin-top:3px}
.agent-mail code{font-size:11px;background:#f5f5f5;border-radius:3px;padding:1px 5px}
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
.agent-by{font-size:12px;color:#999}
.agent-mine{font-size:12px;color:#999}
.agent-publish{display:flex;align-items:center;gap:8px;margin:6px 0 0}
.agent-pub{display:flex;align-items:center;gap:6px;font-size:12px;color:#666;cursor:pointer}
.agent-pub input{width:auto;flex:none;margin:0}
.agent-secret{background:#f5f5f5;padding:10px 12px;font-size:12px;overflow-x:auto;border-radius:6px;margin:0;word-break:break-all}
.agent-input{display:block;width:100%;padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit;margin:0 0 10px}
.agent-scope-pick{border-top:1px solid #eee;padding-top:12px;margin:0 0 14px}
</style>` + chipCSS

// chipCSS is the selection control both agent pages use.
//
// An <input type=checkbox> next to text never lines up: the box sits on the
// text baseline, its height is the browser's rather than the line's, and every
// fix is a different magic number per browser. Hiding the input and styling its
// label removes the alignment problem instead of tuning it — there is nothing
// inline left to misalign.
//
// Shared because /agents was built this way and /agent/new was not, so the same
// choice — which services or tools may this agent reach — was a row of neat
// chips on one page and a column of drifting checkboxes on the other.
// The segmented "where does it run" control moved in here with the chips when
// creating an agent stopped happening in two places: the builder is the one
// form now, so the styles it needs have to travel with it rather than living
// on the roster page that used to own the other copy.
const chipCSS = `<style>
.pick input,.chip input{position:absolute;opacity:0;width:0;height:0}
.pick input:focus-visible+span,.chip input:focus-visible+span{outline:2px solid #111;outline-offset:2px}
.agent-services{display:flex;flex-wrap:wrap;gap:6px}
.chip span{display:block;border:1px solid #ddd;border-radius:999px;padding:6px 12px;
  cursor:pointer;font-size:13px;color:#444;white-space:nowrap}
.chip span:hover{border-color:#bbb}
.chip input:checked+span{background:#111;border-color:#111;color:#fff}
.pick-row{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin:0 0 14px}
.pick span{display:block;height:100%;border:1px solid #ddd;border-radius:8px;padding:10px 12px;
  cursor:pointer;font-size:12px;color:#666;line-height:1.4}
.pick strong{display:block;font-size:13px;color:#111;margin:0 0 2px}
.pick span:hover{border-color:#bbb}
.pick input:checked+span{border-color:#111;background:#fafafa}
.pick input:checked+span strong::after{content:" ✓";color:#0a7d33}
@media only screen and (max-width:600px){.pick-row{grid-template-columns:1fr}}
</style>`

