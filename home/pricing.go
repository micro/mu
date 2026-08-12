package home

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/wallet"
)

func PricingHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	// Everything sits in one centered column so the hero, plan cards and the
	// info cards below share a single width and centre line.
	b.WriteString(`<div style="max-width:760px;margin:0 auto">`)

	// Hero
	b.WriteString(`<div style="text-align:center;padding:24px 0 0">`)
	b.WriteString(`<h2 style="font-size:1.6rem;margin:0 0 8px">Tools for agents</h2>`)
	// One audience, said once. It read "Tools for agents, and for you", and the
	// second half was a hedge that cost both halves their clarity: somebody
	// wiring up an agent could not tell whether this was for them, and somebody
	// looking for an app could not either. The app is how you see the tools
	// work, not a second product being sold alongside them.
	b.WriteString(`<p style="color:#666;font-size:15px;margin:0 0 24px">The everyday internet as tools your agent calls over <a href="/mcp" style="color:#111">MCP</a>, metered per call. The app you sign into is included. <a href="/tools" style="color:#111">Browse the tools →</a> Or self-host for free.</p>`)
	b.WriteString(`</div>`)

	// Plans.
	//
	// They were Free, Starter at £5 and Pro at £10, and they sold credits: £5
	// bought 500 credits, which is £5 of credits, so the tier sold nothing that
	// topping up did not. £10 bought 1,200, which is a 20% discount on the one
	// thing that costs us money — a plan that loses more the better it does.
	//
	// So a credit is 1p on every plan and what a plan sells is scale: how many
	// agents you may run, and whether the channels that spend real money are
	// open. See docs/PRICING.md.
	b.WriteString(`<div class="plans">`)

	type plan struct {
		name, price, sub string
		features         []string
		cta, href        string
		featured         bool
	}
	for _, p := range []plan{
		{
			name: "Free", price: "£0", sub: "500 credits to start, then 100 a month",
			features: []string{
				"Every tool over MCP",
				"1 agent",
				"The web app, included",
				"No card needed",
			},
			cta: "Start free", href: "/signup",
		},
		{
			name: "Pro", price: "£20", sub: "2,000 credits a month",
			features: []string{
				"Everything in Free",
				"5 agents",
				"Send mail, SMS and WhatsApp",
				"Higher rate limits",
			},
			cta: "Get Pro", href: "/signup", featured: true,
		},
		{
			name: "Premium", price: "£100", sub: "10,000 credits a month",
			features: []string{
				"Everything in Pro",
				"25 agents",
				"Highest rate limits",
			},
			cta: "Get Premium", href: "/signup",
		},
	} {
		cls := "card plan"
		if p.featured {
			cls += " plan-featured"
		}
		b.WriteString(`<div class="` + cls + `">`)
		b.WriteString(`<h3 style="margin:0 0 4px">` + p.name + `</h3>`)
		b.WriteString(`<p style="font-size:2rem;font-weight:700;margin:8px 0">` + p.price)
		if p.price != "£0" {
			b.WriteString(`<span style="font-size:14px;font-weight:400;color:#888">/month</span>`)
		}
		b.WriteString(`</p>`)
		b.WriteString(`<p style="color:#666;font-size:14px;margin:0 0 16px">` + html.EscapeString(p.sub) + `</p>`)
		b.WriteString(`<ul style="text-align:left;list-style:none;padding:0;margin:0 0 16px;font-size:14px;line-height:2">`)
		for _, f := range p.features {
			b.WriteString(`<li>&#10003; ` + html.EscapeString(f) + `</li>`)
		}
		b.WriteString(`</ul>`)
		b.WriteString(`<a href="` + p.href + `" class="btn" style="display:block;text-align:center">` +
			html.EscapeString(p.cta) + `</a>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)

	// The ceiling, said before somebody meets it. Credits included in a plan
	// are what the plan costs, not an allowance that renews when it runs out —
	// so the sentence that matters is what happens when it does.
	b.WriteString(`<p style="text-align:center;font-size:14px;color:#666;margin:0 0 24px">` +
		`A credit is 1p on every plan — there is no tier where usage is cheaper. ` +
		`Run out and you top up at the same rate, any amount, or carry on next month. ` +
		`<a href="/pricing#costs" style="color:#111">What a call costs →</a></p>`)

	// Credit costs
	b.WriteString(`<div class="card" id="costs" style="margin:0 0 16px">`)
	b.WriteString(`<h3>Credit costs</h3>`)
	// What is free is the app, not a list of services. The line here used to
	// say "reading the news, markets and weather is free", which was true of
	// the page and not of the tool — the same forecast is included when you
	// look at it and metered when your agent asks for it.
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 8px">1 credit = 1p. The web app is included — you are charged for what your agents call.</p>`)
	// Rendered from wallet.Pricing() so this page and the wallet page can never
	// disagree about what something costs.
	b.WriteString(`<table class="stats-table" style="font-size:14px">`)
	b.WriteString(`<tr><th style="text-align:left">Action</th><th style="text-align:right">Credits</th></tr>`)
	b.WriteString(`<tr><td>Anything you read in the web app</td><td>included</td></tr>`)
	for _, it := range wallet.Pricing() {
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

	// How you actually pay. Two ways, and which applies depends on whether you
	// are a person or a program — the page said "one balance" and never
	// mentioned that an agent does not need a balance, or an account, at all.
	b.WriteString(`<div class="card" style="margin:0 0 16px">`)
	b.WriteString(`<h3>Two ways to pay</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 12px"><strong>People top up with a card.</strong> ` +
		`Credits are bought through Stripe and every priced call — the pages, the assistant, ` +
		`and any agent you have given a token — comes out of the same balance. ` +
		`<a href="/wallet" style="color:#111">Your wallet →</a></p>`)
	if wallet.X402Enabled() {
		b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 8px"><strong>Agents pay per request in USDC, with no account at all.</strong> ` +
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
		b.WriteString(`<p style="text-align:center;font-size:14px;color:#888;margin:16px 0">Already have an account? <a href="/login">Log in</a></p>`)
	}

	b.WriteString(`</div>`) // close centered column

	page := app.RenderHTMLForRequest("Pricing", "Personal AI — plans and pricing", b.String(), r)
	w.Write([]byte(page))
}
