package agent

// What an agent is, for the person who made one.
//
// Clicking an agent opened a chat with it. For one that runs here that is the
// right thing. For one you built to run elsewhere — the whole point of which is
// that something outside calls in with its token — a chat box is the least
// useful half of the page, and the endpoint, the scope and the token were
// nowhere on it. The page even carried a generic "Connect your agent" link
// that pointed at /tools rather than at this agent.
//
// So there is a page saying how to reach one, and clicking an agent that runs
// elsewhere opens it rather than a chat. It is the one page beside the
// conversation, reached from the link in the bar above it — not a tab, because
// a strip of four names for one thing is what the agent surface had and what it
// was rightly called bizarre for.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/agent/micro"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/mail"
)

// ConnectHandler serves /agent/connect: how to reach one agent.
func ConnectHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	a := For(owner, id)

	// Built as one string and wrapped once, so every path opens and closes the
	// column exactly once. An early return that closes a div its caller opened
	// ends the page layout early — see TestMarkupBuildersCloseTheirTags.
	title, desc := "Connect to Micro", "How to reach the default agent"
	body := defaultPanel(app.BaseURL(r))
	switch {
	case a != nil:
		title, desc = "Connect to "+a.Name, "How to reach "+a.Name
		body = connectPanel(a, app.BaseURL(r), auth.CSRFToken(r))
	case Platform(id) != nil && !strings.EqualFold(id, DefaultPlatformAgent):
		// One of this instance's own. Without this every specialist's "How to
		// reach it" fell through to the default panel and said Micro — eleven
		// links to a page about a different agent.
		p := Platform(id)
		title, desc = "Connect to "+p.Name, "How to reach "+p.Name
		body = platformPanel(p, app.BaseURL(r))
	}

	// Back to the roster, not to a conversation.
	//
	// It said "← Back to the conversation" and pointed at /agent — which is
	// wrong twice. Most people arrive here from /agents, having clicked Connect
	// on a row, so "back" sent them somewhere they had not been. And for an
	// agent that runs elsewhere there is no conversation to go back to: it is
	// reached over MCP by something outside this instance, and this product has
	// never claimed you can chat with one here.
	//
	// /agents is where the link on this page comes from and where every agent is
	// listed, so it is both true and the same for everybody.
	back := "/agents"
	// The same picker the conversation has, and on a phone the same sheet: the
	// column is fixed off-screen there, so without the button it would be a
	// panel nothing could open.
	// The secret, on the page that asked for it.
	//
	// Issuing a token posts to /agents, which is where the action lives, and it
	// used to redirect there too — so you pressed a button on this page and
	// were shown a different one. It comes back here now, and the panel that
	// shows a secret once has to be drawn here as well or the round trip ends
	// with the token nowhere.
	notice := ""
	if secret := r.URL.Query().Get("secret"); secret != "" {
		notice = secretPanel(secret, a, app.BaseURL(r))
	} else if msg := strings.TrimSpace(r.URL.Query().Get("error")); msg != "" {
		notice = `<p class="text-error">` + html.EscapeString(msg) + `</p>`
	}

	page := `<div class="chat-layout"><div class="chat-side">` +
		`<div class="chat-pane" id="pane-agents">` + renderAgentsPanel() + `</div></div>` +
		`<div class="chat-main"><p class="conn-back">` +
		// TextLink, not Link: app.Link appends a → because it points at a
		// destination you are going on to, and a back link that reads
		// "← Agents →" is pointing both ways at once.
		app.TextLink("← Agents", back) +
		`<button type="button" class="chat-open-list" onclick="muPane('agents')">Agents</button></p>` +
		`<div style="max-width:820px">` + notice + body + `</div></div></div>` +
		chatLayoutCSS + connectCSS + paneJS +
		`<script>window.muSeedAgent(` + app.JSString(id) + `);</script>`
	app.Respond(w, r, app.Response{Title: title, Description: desc, HTML: page})
}

// defaultPanel is the same page for Micro, which is nobody's roster entry.
//
// The default agent used to have no Connect tab at all, on the reasoning that it
// has no token, no scope and no address of its own. Two of those are wrong: it
// has an address — agent@<domain>, the one nothing has to be remembered for —
// and it reaches every tool, which is a scope and the widest one. Only the token
// is genuinely absent, because Micro is not a separate identity: it is this
// account, so it uses this account's token.
//
// Leaving the tab out meant the tab strip changed shape depending on which agent
// you had selected, and the agent most people are looking at was the one with no
// answer to "how do I reach this".
func defaultPanel(base string) string {
	var b strings.Builder

	b.WriteString(`<p class="lens-lead"><strong>Micro</strong> — Runs here. The default agent: ` +
		`every tool on this instance, no setup, and nothing to remember. It is not a separate ` +
		`identity, so it uses your account rather than a token of its own. ` +
		app.Link("Make one that is", "/agent/new") + `</p>`)

	b.WriteString(`<div class="conn-row"><span class="conn-k">May reach</span>` +
		`<span class="conn-scope wide">everything you can reach</span></div>`)

	b.WriteString(`<div class="conn-row"><span class="conn-k">Endpoint</span>` +
		`<code class="conn-v">` + html.EscapeString(strings.TrimSuffix(base, "/")+"/mcp") + `</code></div>`)

	if addr := mail.SharedAgentAddress(); addr != "" {
		// The address, with no paragraph under it. This carried three lines
		// about verified addresses, what is filed rather than answered, and
		// which conversation list it shows up in — rules, on a page whose job
		// is to hand you the four things you copy somewhere else.
		b.WriteString(`<div class="conn-row"><span class="conn-k">Email</span>` +
			`<code class="conn-v">` + html.EscapeString(addr) + `</code></div>`)
	}

	b.WriteString(`<div class="conn-row"><span class="conn-k">Token</span>` +
		`<span class="conn-v">Your account's. ` + app.TextLink("Issue one", "/token") +
		` and anything holding it reaches every tool you can — which is why an agent you ` +
		`hand to somebody else should be ` + app.TextLink("its own", "/agent/new") +
		`, with a scope.</span></div>`)

	b.WriteString(`<h3 class="conn-head">Configuration</h3>`)
	b.WriteString(`<pre class="conn-pre">` + html.EscapeString(`{
  "mcpServers": {
    "mu": {
      "url": "`+strings.TrimSuffix(base, "/")+`/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}`) + `</pre>`)
	b.WriteString(`<p class="conn-note">Claude Desktop takes the URL on its own — Settings → ` +
		`Connectors → Add custom connector — and signs you in instead of using a token. ` +
		app.Link("Full setup", "/tools") + `</p>`)

	return b.String()
}

// connectPanel is everything you need to point something at this agent: what it
// may reach, where to send it, what to write to, and whether it has a
// credential.
func connectPanel(a *Agent, base, csrf string) string {
	var b strings.Builder

	// What it may reach, first. The scope is the thing that makes handing out a
	// token safe, so it goes above the token rather than below it.
	scope := "everything you can reach"
	cls := "conn-scope wide"
	if !a.Unscoped() {
		labels := make([]string, 0, len(a.Services))
		for _, s := range a.Services {
			labels = append(labels, service.Label(s))
		}
		scope = strings.Join(labels, ", ")
		cls = "conn-scope"
	}
	kind := "Runs here"
	kindNote := "This instance executes its standing instruction. It needs no credential " +
		"unless you want to reach it from outside."
	if a.Kind != Hosted {
		kind = "Runs elsewhere"
		kindNote = "Claude, Cursor or your own program calls in with its token."
	}
	b.WriteString(`<p class="lens-lead"><strong>` + html.EscapeString(a.Name) + `</strong> — ` +
		html.EscapeString(kind) + `. ` + html.EscapeString(kindNote) + `</p>`)

	b.WriteString(`<div class="conn-row"><span class="conn-k">May reach</span>` +
		`<span class="` + cls + `">` + html.EscapeString(scope) + `</span></div>`)

	// The endpoint carries the scope in the query string, so pointing a client
	// at it lists exactly what this agent may call.
	b.WriteString(`<div class="conn-row"><span class="conn-k">Endpoint</span>` +
		`<code class="conn-v">` + html.EscapeString(a.Endpoint(base)) + `</code></div>`)

	// The address, and who it listens to. Knowing the address is not permission
	// to use it — mail from anyone else is filed and ignored — and a reader who
	// does not know that reports the silence as a bug.
	//
	// The shared address goes here too. Nobody remembers a plus-address from a
	// phone; the point of agent@ is that there is nothing to remember, and an
	// address nobody is told about is an address nobody uses.
	if addr := a.Address(); addr != "" {
		// Without a mail domain this is a handle rather than an address, so the
		// row has to say what it is good for. Everything below about verified
		// senders and filtering is about mail arriving from outside, which on
		// such an instance never happens.
		if !mail.Reachable() {
			b.WriteString(`<div class="conn-row"><span class="conn-k">Message it</span>` +
				`<span class="conn-v"><code>` + html.EscapeString(addr) + `</code><br>` +
				`<span class="conn-sub">From anyone on this instance. There is no mail domain ` +
				`configured, so nothing from outside can reach it — an operator sets ` +
				`<code>MAIL_DOMAIN</code> to change that.</span></span></div>`)
		} else {
			// The address, and nothing under it.
			//
			// This carried two paragraphs: who it answers and what is filed
			// instead, then a second address with its own explanation. Both are
			// rules rather than things to copy, and this page is a list of four
			// things you take somewhere else — an endpoint, an address, a
			// scope, a token. Where the rules belong is /help; where the other
			// address belongs is the page about the other agent.
			b.WriteString(`<div class="conn-row"><span class="conn-k">Email</span>` +
				`<code class="conn-v">` + html.EscapeString(addr) + `</code></div>`)
		}
	}

	// The token. A secret is shown once and never again, so this reports state
	// rather than pretending it can show you one.
	if a.TokenID == "" {
		b.WriteString(`<div class="conn-row"><span class="conn-k">Token</span>` +
			`<span class="conn-v">None yet. ` +
			fmt.Sprintf(`<form method="POST" action="/agents" style="display:inline">`+
				`<input type="hidden" name="_csrf" value="%s">`+
				`<input type="hidden" name="action" value="token">`+
				`<input type="hidden" name="id" value="%s">`+
				`<input type="hidden" name="back" value="/agent/connect?id=%s">`+
				`<button type="submit" class="link-button">Issue one</button></form>`,
				html.EscapeString(csrf), html.EscapeString(a.ID), html.EscapeString(a.ID)) +
			`</span></div>`)
	} else {
		used := "not called yet"
		if !a.LastUsed.IsZero() {
			used = "last called " + a.LastUsed.Local().Format("2 Jan 15:04")
		}
		b.WriteString(`<div class="conn-row"><span class="conn-k">Token</span>` +
			`<span class="conn-v">Issued · ` + html.EscapeString(used) +
			`. The secret was shown once and is stored hashed — ` +
			fmt.Sprintf(`<form method="POST" action="/agents" style="display:inline" `+
				`onsubmit="return confirm('Replace this token? Anything using the old one stops working.')">`+
				`<input type="hidden" name="_csrf" value="%s">`+
				`<input type="hidden" name="action" value="token">`+
				`<input type="hidden" name="id" value="%s">`+
				`<input type="hidden" name="back" value="/agent/connect?id=%s">`+
				`<button type="submit" class="link-button">replace it</button></form>`,
				html.EscapeString(csrf), html.EscapeString(a.ID), html.EscapeString(a.ID)) +
			` to get a new one.</span></div>`)
	}

	// How to actually wire it up, with this agent's own endpoint in it rather
	// than the generic one /tools shows.
	b.WriteString(`<h3 class="conn-head">Configuration</h3>`)
	b.WriteString(`<pre class="conn-pre">` + html.EscapeString(`{
  "mcpServers": {
    "`+strings.ToLower(a.Name)+`": {
      "url": "`+a.Endpoint(base)+`",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}`) + `</pre>`)
	b.WriteString(`<p class="conn-note">Claude Desktop takes the URL on its own — Settings → ` +
		`Connectors → Add custom connector — and signs you in instead of using a token. ` +
		app.Link("Full setup", "/tools") + `</p>`)

	return b.String()
}

const connectCSS = `<style>
.conn-row{display:flex;flex-wrap:wrap;gap:4px 14px;align-items:baseline;padding:9px 0;
  border-bottom:1px solid var(--border-color,#eee)}
.conn-k{flex:0 0 110px;font-size:12px;color:var(--text-muted,#999)}
.conn-v{flex:1 1 300px;min-width:0;font-size:13px;overflow-wrap:anywhere}
code.conn-v{background:var(--hover-background,#f5f5f5);border-radius:4px;padding:2px 7px}
.conn-sub{display:inline-block;margin-top:4px;font-size:12px;color:var(--text-muted,#888)}
.conn-scope{flex:1 1 300px;font-size:13px;color:#0a7d33}
.conn-scope.wide{color:#a86400}
.conn-head{font-size:15px;margin:22px 0 8px}
.conn-pre{background:var(--hover-background,#f5f5f5);border-radius:8px;padding:12px 14px;
  font-size:12px;overflow-x:auto;margin:0}
.conn-note{font-size:13px;color:var(--text-muted,#666);margin:10px 0 0}
.conn-back{display:flex;align-items:center;gap:12px;font-size:13px;margin:0 0 14px}
/* The rows are label-and-value pairs; below 600px the label column eats most of
   a phone's width and the value — an endpoint, an address, the thing you came to
   copy — wraps to a column two words wide. */
@media(max-width:600px){
  .conn-row{display:block;padding:10px 0}
  .conn-k{display:block;flex:none;margin-bottom:3px}
  .conn-v,.conn-scope{display:block;flex:none}
  .conn-pre{font-size:11px}
}
</style>`

// platformPanel is how to reach one of this instance's own agents.
//
// Shorter than an agent of your own has, and the difference is the point: there
// is no token, no scope to set and nothing to create. It is here, it answers at
// an address, and it reaches the services it was built for.
func platformPanel(a *micro.Agent, base string) string {
	var b strings.Builder
	b.WriteString(`<p class="lens-lead"><strong>` + html.EscapeString(a.Name) +
		`</strong> — Runs here. ` + html.EscapeString(a.Description) + `. Provided by this ` +
		`instance, so there is nothing to create and no token to hold: it answers as your ` +
		`account, against your credits. ` + app.Link("Make one of your own", "/agent/new") + `</p>`)

	b.WriteString(`<div class="conn-row"><span class="conn-k">May reach</span>` +
		`<span class="conn-scope">` + html.EscapeString(toolWords(a.Tools)) + `</span></div>`)

	if addr := PlatformAddress(a.ID); addr != "" {
		b.WriteString(`<div class="conn-row"><span class="conn-k">Write to it</span>` +
			`<span class="conn-v"><code>` + html.EscapeString(addr) + `</code><br>` +
			`<span class="conn-sub">From your verified address. The tag names the agent ` +
			`rather than you, so it is one thing to remember — and it answers in the ` +
			`thread, which turns up in your inbox.</span></span></div>`)
	}

	b.WriteString(`<div class="conn-row"><span class="conn-k">Endpoint</span>` +
		`<code class="conn-v">` + html.EscapeString(strings.TrimSuffix(base, "/")+"/mcp") + `</code></div>`)

	b.WriteString(`<div class="conn-row"><span class="conn-k">Talk to it</span>` +
		`<span class="conn-v">` + app.TextLink(base+"/agent/"+html.EscapeString(a.ID),
		"/agent/"+html.EscapeString(a.ID)) + `</span></div>`)
	return b.String()
}
