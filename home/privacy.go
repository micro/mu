package home

import (
	"net/http"
	"strings"

	"mu/internal/app"
)

// PrivacyHandler serves /privacy.
//
// It exists because it was missing and needed: every MCP directory submission
// asks for a privacy policy URL, and Mu had none. It is written to be true
// rather than to be safe — this instance runs a mail server and an agent, so it
// holds real correspondence and real questions, and saying "we care about your
// privacy" over the top of that would be worth less than saying what is stored.
//
// Kept in code rather than in docs/ because it is a site page, not developer
// documentation: a person looking for it is not reading about architecture.
func PrivacyHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	b.WriteString(`<div style="max-width:680px;margin:0 auto">`)
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Privacy</h2>`)
	b.WriteString(`<p class="text-sm text-muted">What this instance stores, why, and what it never does.</p>`)
	b.WriteString(`</div>`)

	section := func(title string, paras ...string) {
		b.WriteString(`<div class="card"><h3>` + title + `</h3>`)
		for _, p := range paras {
			b.WriteString(`<p>` + p + `</p>`)
		}
		b.WriteString(`</div>`)
	}

	section("What we don't do",
		`No advertising. No tracking pixels, no third-party analytics, no cross-site
		 identifiers. Nothing you do here is profiled to sell you something, and no
		 data is sold or shared with a data broker — there is no business model here
		 that would want it.`,
		`Your content is not used to train anyone's model.`)

	section("What is stored",
		`<b>Your account</b> — a username, a password hash or passkey, and an email
		 address if you gave one. Signing in with Google stores the email and name
		 Google returns, nothing else.`,
		`<b>Your mail</b> — this instance runs an SMTP server, so messages sent to
		 and from your address are stored here in order to be an inbox at all.
		 They are readable by you, and by whoever operates the instance, in the
		 same way any mail server works.`,
		`<b>What you make</b> — posts, blogs, files, contacts, calendar entries,
		 stored records. Private unless you make them public.`,
		`<b>Your credits</b> — a balance and a transaction history, so charges can
		 be explained.`,
		`<b>Operational logs</b> — requests, errors and IP addresses, kept short-term
		 to run the service and stop abuse.`)

	section("Agents and third parties",
		`Some tools reach outside this instance, and when they do, the request goes
		 to that provider: web search to Brave, places and travel time to Google,
		 image generation and most model calls to Atlas Cloud, with Anthropic for
		 the premium model tier. Those requests carry the query, not your identity.`,
		`A question you ask the agent is sent to a model provider to be answered.
		 If that matters for something you are about to ask, it is worth knowing
		 before you ask it.`)

	section("Tokens and connected agents",
		`A personal access token, or a client you signed in through the MCP
		 authorization flow, acts as you: it reaches the same tools and the same
		 data. Revoke either at <a href="/token">/token</a>. Anything an agent did
		 with a token is attributed to your account, which is why
		 <code>mail_send</code> is available only to a signed-in account and never
		 to an anonymous caller.`)

	section("Deleting things",
		`Delete individual items — a file, a record, a post, a contact — with the
		 tool or page that made them. To close an account and remove what it holds,
		 mail the operator of the instance. On micro.mu that is
		 <a href="mailto:admin@micro.mu">admin@micro.mu</a>.`)

	section("Self-hosting",
		`Mu is one Go binary and the source is <a href="https://github.com/micro/mu">public</a>.
		 Run your own instance and none of the above involves us at all: the data is
		 on your disk, and you are the operator this page is describing.`)

	b.WriteString(`</div>`)

	w.Write([]byte(app.RenderHTMLForRequest("Privacy",
		"What this instance stores, why, and what it never does", b.String(), r)))
}
