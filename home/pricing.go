package home

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/account"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/x402"
)

func PricingHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	// One column, anchored where every other page's content is anchored.
	//
	// It was centred — margin:0 auto inside #content, which is 1400px wide and
	// left-aligned — so the 760px column sat in the middle of the page while
	// the "Pricing" heading above it stayed at the left edge. The hero read as
	// belonging to some other page: nothing lined up with the one heading the
	// visitor had just read. A column that starts where the title starts costs
	// nothing and puts them on the same line.
	b.WriteString(`<div style="max-width:760px">`)

	// Hero
	b.WriteString(`<div style="padding:16px 0 0">`)
	b.WriteString(`<h2 style="font-size:1.6rem;margin:0 0 8px">Tools for agents</h2>`)
	// One audience, said once. It read "Tools for agents, and for you", and the
	// second half was a hedge that cost both halves their clarity: somebody
	// wiring up an agent could not tell whether this was for them, and somebody
	// looking for an app could not either. The app is how you see the tools
	// work, not a second product being sold alongside them.
	b.WriteString(`<p style="color:#666;font-size:15px;margin:0 0 12px">The everyday internet as tools your agent calls over <a href="/mcp" style="color:#111">MCP</a>, metered per call. The app you sign into is included.</p>`)
	// The two links are their own line. Run on from the sentence above they read
	// as more prose and get skipped — a call to action that is the tail of a
	// paragraph is not one.
	b.WriteString(fmt.Sprintf(`<p style="color:#666;font-size:15px;margin:0 0 12px">`+
		`One price, one way: <strong>a credit is 1p</strong>, top up any amount, and all %d tools `+
		`are there from the first penny. No plan, no tier, nothing held back for a bigger one.</p>`,
		api.ToolCount()))
	b.WriteString(`<p style="color:#666;font-size:15px;margin:0 0 24px"><a href="/tools" style="color:#111">Browse the tools →</a> &nbsp;·&nbsp; Or self-host for free.</p>`)

	b.WriteString(`</div>`)

	// Credit costs
	b.WriteString(`<div class="card" id="costs" style="margin:0 0 16px">`)
	b.WriteString(`<h3>Credit costs</h3>`)
	// What is free is the app, not a list of services. The line here used to
	// say "reading the news, markets and weather is free", which was true of
	// the page and not of the tool — the same forecast is included when you
	// look at it and metered when your agent asks for it.
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 8px">1 credit = 1p. The web app is included — you are charged for what your agents call.</p>`)
	// Rendered from account.Pricing() so this page and the account page can never
	// disagree about what something costs.
	b.WriteString(`<table class="stats-table" style="font-size:14px">`)
	b.WriteString(`<tr><th style="text-align:left">Action</th><th style="text-align:right">Credits</th></tr>`)
	b.WriteString(`<tr><td>Anything you read in the web app</td><td>included</td></tr>`)
	for _, it := range account.Pricing() {
		cost := fmt.Sprintf("%d", it.Cost)
		if it.Cost == 0 {
			cost = "free"
		}
		b.WriteString(`<tr><td>` + html.EscapeString(it.Description) + `</td><td>` + cost + `</td></tr>`)
	}
	b.WriteString(`<tr><td>Using a paid app</td><td>set by its author</td></tr>`)
	b.WriteString(`</table>`)
	b.WriteString(`<p style="font-size:13px;color:#888;margin:8px 0 0">Most apps are free. Paid ones show their price before you run them; the author keeps 90%.</p>`)
	b.WriteString(`</div>`)

	// How you actually pay. "Can also", because the paragraph above has just
	// explained that an agent spends its owner's balance through a token — and
	// then this one said agents pay per request in USDC, which reads as a
	// correction rather than a second option. Both are true at once: give an
	// agent a token and it draws on your credits, or let it pay its own way and
	// it needs nothing from you.
	//
	// There were plan cards above this once, selling agents, send volume and
	// concurrency for £20 a month. They went because they sold three limits and
	// a credit at par, which is nothing a top-up does not do — and because every
	// one of them was a moving part between a customer and paying us. The limits
	// they sold are still there; they lift when an account becomes accountable,
	// which is a verified address or money on the balance, and neither needs a
	// subscription.
	b.WriteString(`<div class="card" style="margin:0 0 16px">`)
	b.WriteString(`<h3>How you pay</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 12px"><strong>People top up with a card.</strong> ` +
		`Credits are bought through Stripe and every priced call — the pages, the assistant, ` +
		`and any agent you have given a token — comes out of the same balance. ` +
		`<a href="/account#balance" style="color:#111">Your balance →</a></p>`)
	if x402.Enabled() {
		b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 8px"><strong>Agents can also pay using USDC, with no account.</strong> ` +
			`A priced call with no credentials answers <code>402 Payment Required</code> with an ` +
			`<a href="https://x402.org" style="color:#111">x402</a> challenge naming the price and where to send it. ` +
			`Pay, retry, get the answer — no signup, no card, no key to rotate.</p>`)
		b.WriteString(`<pre style="font-size:12px;background:#fafafa;border:1px solid #eee;border-radius:6px;padding:10px;overflow-x:auto;margin:0">` +
			`curl -X POST ` + html.EscapeString(app.BaseURL(r)) + `/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call",
       "params":{"name":"web_search","arguments":{"query":"x402"}}}'</pre>`)
	}
	b.WriteString(`</div>`)

	// Self-host
	b.WriteString(`<div class="card" style="margin:0 0 16px">`)
	b.WriteString(`<h3>Self-host</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666">Run your own instance with no limits. Single Go binary, bring your own AI provider. With no Stripe keys and no x402 address an instance cannot charge anybody, so nothing is metered and every tool is free.</p>`)
	b.WriteString(`<p style="font-size:14px"><a href="https://github.com/micro/mu">github.com/micro/mu</a></p>`)
	b.WriteString(`</div>`)

	// Login link (only when not logged in)
	if sess, _ := auth.TrySession(r); sess == nil {
		b.WriteString(`<p style="font-size:14px;color:#888;margin:16px 0">Already have an account? <a href="/login">Log in</a></p>`)
	}

	b.WriteString(`</div>`) // close centered column

	page := app.RenderHTMLForRequest("Pricing", "What it costs and how you pay", b.String(), r)
	w.Write([]byte(page))
}
