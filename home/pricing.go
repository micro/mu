package home

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/wallet"
)

// money renders pence as £10 — plans are whole pounds, and a plan that is not
// would be a pricing decision rather than a formatting one.
func money(pence int) string {
	if pence%100 == 0 {
		return "£" + thousands(pence/100)
	}
	return fmt.Sprintf("£%d.%02d", pence/100, pence%100)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// thousands puts separators in, so 10000 credits reads as 10,000.
func thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

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
	//
	// And there is no free column. Every call here spends somebody's money —
	// Atlas for inference, Brave for search, Google for places and routes,
	// Twilio for a text — so a tier that costs nothing is a tier that runs at a
	// loss for every account that uses it, and the ones that use it most cost
	// the most. Self-hosting is the free option and it is a real one: the binary
	// is AGPL, it wants your API keys rather than ours, and an instance with no
	// Stripe and no x402 cannot charge anybody. What is sold here is not
	// software. It is not having to hold thirty accounts.
	b.WriteString(`<div class="plans">`)

	// Pay as you go is a column, not an absence.
	//
	// It is where the third paid tier was, and it is the answer to "I do not
	// want to gate": every read tool is here, unmetered by any plan, bought at
	// the same penny a credit as everywhere else. What it does not carry is
	// volume on the three operations that leave the building, and that is a
	// wall against abuse rather than against a customer — what an account sends
	// under our domain and our number is the one cost a balance cannot make
	// whole.
	b.WriteString(`<div class="card plan">`)
	b.WriteString(`<h3 style="margin:0 0 4px">Pay as you go</h3>`)
	b.WriteString(`<p style="font-size:2rem;font-weight:700;margin:8px 0">1p` +
		`<span style="font-size:14px;font-weight:400;color:#888">/credit</span></p>`)
	b.WriteString(`<p style="color:#666;font-size:14px;margin:0 0 16px">Top up any amount</p>`)
	b.WriteString(`<ul style="text-align:left;list-style:none;padding:0;margin:0 0 16px;font-size:14px;line-height:2">`)
	b.WriteString(`<li>&#10003; Every read tool</li>`)
	b.WriteString(`<li>&#10003; The web app, included</li>`)
	b.WriteString(`<li>&#10003; 1 agent</li>`)
	b.WriteString(`<li>&#10003; No subscription</li>`)
	b.WriteString(`</ul>`)
	b.WriteString(`<a href="/signup" class="btn" style="display:block;text-align:center">Start</a>`)
	b.WriteString(`</div>`)

	// Rendered from wallet.Plans(), which is what /wallet actually charges for,
	// so this page cannot advertise a plan nobody can buy.
	for _, p := range wallet.Plans() {
		cls := "card plan"
		if p.Featured {
			cls += " plan-featured"
		}
		b.WriteString(`<div class="` + cls + `">`)
		b.WriteString(`<h3 style="margin:0 0 4px">` + html.EscapeString(p.Name) + `</h3>`)
		b.WriteString(`<p style="font-size:2rem;font-weight:700;margin:8px 0">` +
			html.EscapeString(money(p.Price)) +
			`<span style="font-size:14px;font-weight:400;color:#888">/month</span></p>`)
		b.WriteString(`<p style="color:#666;font-size:14px;margin:0 0 16px">` +
			html.EscapeString(thousands(p.Credits)+" credits a month") + `</p>`)
		b.WriteString(`<ul style="text-align:left;list-style:none;padding:0;margin:0 0 16px;font-size:14px;line-height:2">`)
		for _, f := range p.Features {
			b.WriteString(`<li>&#10003; ` + html.EscapeString(f) + `</li>`)
		}
		// Written from the field the limit is enforced from, which is the only
		// arrangement where the card cannot be wrong.
		//
		// The post rate is not here, though a plan does raise it. It is an
		// abuse control on writing to the social side — the defence against a
		// runaway bot filling the timeline — and this is an MCP server, where
		// what a buyer means by a rate limit is how hard their agent may call
		// tools. Advertising "300 posts an hour" answers a question nobody
		// asked and implies we are metering something we are not. A limit and
		// a feature are different things, and only one of them belongs on a
		// price card.
		b.WriteString(`<li>&#10003; ` + html.EscapeString(
			fmt.Sprintf("%d agent%s", p.Agents, plural(p.Agents))) + `</li>`)
		// The send caps, from the same map the gate reads. These are the third
		// thing a plan sells and the only one whose cost to us is not covered
		// by the credits — what a bad month spends is a domain's or a number's
		// reputation, and no balance repairs that.
		for _, l := range []struct {
			op, noun string
		}{
			{quota.OpExternalEmail, "emails"},
			{quota.OpSMSSend, "texts"},
			{quota.OpWhatsAppSend, "WhatsApp chats"},
		} {
			if n, ok := p.LimitFor(l.op); ok {
				b.WriteString(`<li>&#10003; ` + html.EscapeString(
					fmt.Sprintf("%s %s a day", thousands(n), l.noun)) + `</li>`)
			}
		}
		b.WriteString(`</ul>`)
		b.WriteString(`<a href="/signup" class="btn" style="display:block;text-align:center">Get ` +
			html.EscapeString(p.Name) + `</a>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)

	// The ceiling, said before somebody meets it. Credits included in a plan
	// are what the plan costs, not an allowance that renews when it runs out —
	// so the sentence that matters is what happens when it does.
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 24px">` +
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
		b.WriteString(`<p style="font-size:14px;color:#888;margin:16px 0">Already have an account? <a href="/login">Log in</a></p>`)
	}

	b.WriteString(`</div>`) // close centered column

	page := app.RenderHTMLForRequest("Pricing", "Personal AI — plans and pricing", b.String(), r)
	w.Write([]byte(page))
}
