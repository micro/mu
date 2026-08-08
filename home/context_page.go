package home

// What your agents know about you.
//
// The agent has been keeping notes. agent/run.go extracts facts from what you
// say and memory.Set stores them; main.go reads them back into the system
// prompt of every question you ask. Fifty entries, persisted to disk, injected
// into every turn — and until this page there was no way to see one, correct
// one, or delete one. Not a settings screen with the option buried: no UI at
// all, anywhere.
//
// That is the thing worth fixing. A product that remembers you and cannot show
// you what it remembers is asking for trust it has not earned, and the fix is
// not a policy paragraph, it is a list with a delete button next to each row.
//
// The page is deliberately three sections and no more. What it remembers, what
// it watches, and what it can go and look up — which is the difference between
// something the agent carries into every conversation, something you chose to
// put in front of it, and something it fetches only when a question needs it.
// Those three are genuinely different, and confusing them is how "context"
// becomes a drawer for anything account-shaped.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/memory"
)

// ContextHandler serves /context.
func ContextHandler(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "forget":
			memory.Delete(owner, r.FormValue("key"))
		case "forget-all":
			memory.Clear(owner)
		}
		http.Redirect(w, r, "/context", http.StatusSeeOther)
		return
	}

	entries := memory.All(owner)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"memory": entries})
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(`<div style="max-width:720px">`)
	b.WriteString(`<p class="lens-lead">What your agents know. Some of it they remember on their ` +
		`own, some of it you put in front of them, and the rest they go and look up when a ` +
		`question needs it.</p>`)

	// ── What it remembers ───────────────────────────────────────
	b.WriteString(`<h3 class="ctx-head">What it remembers</h3>`)
	b.WriteString(`<p class="ctx-sub">Picked up from your conversations and read back into every ` +
		`question you ask. Delete anything that is wrong or that you would rather it did not keep.</p>`)
	if len(entries) == 0 {
		b.WriteString(`<p class="ctx-empty">Nothing yet. Tell an agent something worth keeping — ` +
			`"remember that I'm in London" — and it will appear here.</p>`)
	} else {
		b.WriteString(`<div class="ctx-list">`)
		for _, e := range entries {
			b.WriteString(`<div class="ctx-row"><div style="flex:1;min-width:0">` +
				`<span class="ctx-key">` + html.EscapeString(e.Key) + `</span>` +
				`<div class="ctx-val">` + html.EscapeString(e.Value) + `</div>` +
				`<div class="ctx-when">learned ` + html.EscapeString(app.TimeAgo(e.CreatedAt)) + `</div></div>` +
				`<form method="POST" action="/context" style="margin:0">` +
				`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">` +
				`<input type="hidden" name="action" value="forget">` +
				`<input type="hidden" name="key" value="` + html.EscapeString(e.Key) + `">` +
				`<button type="submit" class="ctx-forget">Forget</button></form></div>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`<form method="POST" action="/context" style="margin:10px 0 0" ` +
			`onsubmit="return confirm('Forget everything it has learned about you?')">` +
			`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">` +
			`<input type="hidden" name="action" value="forget-all">` +
			`<button type="submit" class="ctx-forget">Forget everything</button></form>`)
	}

	// ── What it watches ─────────────────────────────────────────
	// ── What it watches ─────────────────────────────────────────
	//
	// The cards themselves, not a list of their names. They were on Home, which
	// docs/PRODUCT.md calls "the world's content where your own should be" —
	// Home is the console, this is the context. And a page about what an agent
	// reads should show what it would read, not chips naming it.
	b.WriteString(`<h3 class="ctx-head">What it watches</h3>`)
	b.WriteString(`<p class="ctx-sub">Live from the services you keep an eye on. These are not ` +
		`read automatically — they go in only when you turn on live context above the agent ` +
		`input, because context costs tokens on every turn and most questions have nothing to ` +
		`do with them. ` + app.Link("Choose what to watch", "/home?cards=1") + `</p>`)
	b.WriteString(CardsHTML(r, acc))

	// ── What it can look up ─────────────────────────────────────
	//
	// Linked rather than listed. Naming a curated handful here would be a
	// second list to keep in step with the real one, and the boundary it
	// implies is not real: mail is something an agent looks up and also
	// something you open and read yourself.
	//
	// It points at Tools, not Services. Looking something up is a call, and a
	// call is a tool; Services is the same capabilities seen from the other
	// side, as things this instance runs and you can open. Sending someone to
	// the back end to find out what their agent can fetch is one hop wrong.
	b.WriteString(`<h3 class="ctx-head">What it can look up</h3>`)
	b.WriteString(`<p class="ctx-sub">Everything an agent may reach it fetches when a question ` +
		`needs it, not before — your mail, your records, your files, the news, the web. What any ` +
		`one agent may reach is the scope you gave it. ` +
		app.Link("See every tool", "/tools") + `</p>`)

	b.WriteString(`</div>` + contextCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Context",
		"What your agents know about you, and what they can look up", b.String(), r)))
}

const contextCSS = `<style>
.lens-lead{color:#666;font-size:14px;margin:0 0 18px;max-width:640px}
.ctx-head{font-size:15px;margin:26px 0 4px}
.ctx-sub{color:#666;font-size:13px;margin:0 0 10px;max-width:640px}
.ctx-empty{color:#888;font-size:14px}
.ctx-list{display:flex;flex-direction:column;gap:8px}
.ctx-row{display:flex;align-items:center;gap:12px;border:1px solid #eee;border-radius:8px;padding:10px 14px}
.ctx-key{font-weight:600;font-size:14px}
.ctx-val{font-size:14px;color:#333;margin-top:2px}
.ctx-when{font-size:12px;color:#999;margin-top:3px}
.ctx-forget{background:none;border:0;color:#999;font-size:13px;cursor:pointer;padding:0}
.ctx-forget:hover{color:#b00;text-decoration:underline}
.ctx-chips{display:flex;flex-wrap:wrap;gap:6px}
.ctx-chip{display:block;border:1px solid #ddd;border-radius:999px;padding:5px 12px;font-size:13px;color:#444}
</style>`
