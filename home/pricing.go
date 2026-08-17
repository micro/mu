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

// tierSpec is one of the three ways to have this.
//
// Not three sizes of one plan — the tiers this page used to carry were three
// limits and a credit at par, and they went because none of them was anything a
// top-up did not already do. These are three different relationships: our
// instance metered per call, your instance for nothing, or your instance run by
// us. What differs is who operates it, which is the only axis worth a column.
type tierSpec struct {
	Name   string
	Price  string
	Unit   string
	Lead   string
	Points []string
	Action string
	Href   string
	// Highlight marks the one we would rather you took. Exactly one, and the
	// honest one: a page where every column is emphasised has emphasised none.
	Highlight bool
}

func tier(s tierSpec) string {
	cls := "tier"
	if s.Highlight {
		cls += " tier-on"
	}
	var b strings.Builder
	b.WriteString(`<div class="` + cls + `">`)
	b.WriteString(`<div class="tier-name">` + html.EscapeString(s.Name) + `</div>`)
	b.WriteString(`<div class="tier-price">` + html.EscapeString(s.Price))
	if s.Unit != "" {
		b.WriteString(`<span class="tier-unit">` + html.EscapeString(s.Unit) + `</span>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<p class="tier-lead">` + html.EscapeString(s.Lead) + `</p>`)
	b.WriteString(`<ul class="tier-points">`)
	for _, p := range s.Points {
		b.WriteString(`<li>` + html.EscapeString(p) + `</li>`)
	}
	b.WriteString(`</ul>`)
	b.WriteString(`<a class="tier-act" href="` + html.EscapeString(s.Href) + `">` +
		html.EscapeString(s.Action) + `</a>`)
	b.WriteString(`</div>`)
	return b.String()
}

const tiersCSS = `<style>
.tiers{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;margin:0 0 28px;align-items:stretch}
.tier{display:flex;flex-direction:column;border:1px solid var(--card-border,#e8e8e8);border-radius:10px;
  padding:18px 18px 16px;background:var(--card-background,#fff)}
.tier-on{border-color:var(--text-primary,#111);box-shadow:0 1px 0 var(--text-primary,#111)}
.tier-name{font-size:13px;font-weight:600;letter-spacing:.02em;color:var(--text-muted,#666)}
.tier-price{font-size:1.5rem;font-weight:600;margin:6px 0 10px;color:var(--text-primary,#111)}
.tier-unit{font-size:13px;font-weight:400;color:var(--text-muted,#888);margin-left:6px}
.tier-lead{font-size:13.5px;line-height:1.55;color:var(--text-secondary,#555);margin:0 0 12px}
.tier-points{list-style:none;padding:0;margin:0 0 16px;font-size:13px;color:var(--text-secondary,#555)}
.tier-points li{padding:4px 0 4px 18px;position:relative;line-height:1.45}
.tier-points li::before{content:"·";position:absolute;left:6px;color:#bbb;font-weight:700}
.tier-act{margin-top:auto;display:block;text-align:center;padding:9px 12px;border-radius:6px;
  border:1px solid var(--border-color,#ddd);color:var(--text-primary,#111);text-decoration:none;
  font-size:14px;font-weight:600;overflow-wrap:anywhere}
.tier-act:hover{border-color:#999}
.tier-on .tier-act{background:var(--text-primary,#111);border-color:var(--text-primary,#111);color:#fff}
/* One column on a phone, in the order they are written: try it, run it, or have
   it run. A three-column grid at 380px wide is three unreadable columns. */
@media(max-width:820px){.tiers{grid-template-columns:1fr;gap:12px}}
</style>`

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
	b.WriteString(fmt.Sprintf(`<p style="color:#666;font-size:15px;margin:0 0 20px">`+
		`The everyday internet as tools your agent calls over <a href="/mcp" style="color:#111">MCP</a> — `+
		`all %d of them, one account instead of a hundred. Use ours by the call, run your own for `+
		`nothing, or have us run one for you.</p>`, api.ToolCount()))
	b.WriteString(`</div>`)

	// Three ways to have it, which is three different relationships rather than
	// three sizes of the same one.
	//
	// It was one column of cards — costs, then how you pay, then a note about
	// self-hosting — which reads as a price list for a single product, and the
	// only thing a visitor could do with it was top up. The two options that are
	// not "pay us per call" were a paragraph and a link at the bottom of the
	// page. If somebody wants an instance of their own, or wants one run for
	// them, that has to be visible at the same moment as the price.
	b.WriteString(`<div class="tiers">`)

	b.WriteString(tier(tierSpec{
		Name:  "Pay as you go",
		Price: "1p",
		Unit:  "a credit",
		Lead:  "Use this instance. Top up any amount and every tool is there from the first penny — no plan, no tier, nothing held back for a bigger one.",
		Points: []string{
			"All " + fmt.Sprintf("%d", api.ToolCount()) + " tools",
			"The web app included",
			"Agents can pay per call in USDC, with no account at all",
			"Stop any time — credits do not expire",
		},
		Action: "Top up", Href: "/account/topup",
	}))

	b.WriteString(tier(tierSpec{
		Name:  "Self-host",
		Price: "Free",
		Unit:  "for ever",
		Lead:  "Run your own instance. One Go binary, your machine, your data — and with no Stripe keys and no x402 address it cannot charge anybody, so nothing is metered.",
		Points: []string{
			"Every tool, no limits",
			"Bring your own AI provider",
			"Your own mail domain and agent addresses",
			"Charge for your instance and the money is yours",
		},
		Action: "Get the code", Href: "https://github.com/micro/mu",
	}))

	b.WriteString(tier(tierSpec{
		Name:  "Hosted",
		Price: "Talk to us",
		Unit:  "",
		Lead:  "Your own instance, run by us. Your domain, your mail, your agents, kept up and backed up — the self-hosted product without the hosting.",
		Points: []string{
			"A dedicated instance on your own domain",
			"Mail, agents and tools set up for you",
			"Backups, upgrades and monitoring",
			"Support with an actual person behind it",
		},
		Action: "support@micro.mu", Href: "mailto:support@micro.mu?subject=Hosted%20Mu",
		Highlight: true,
	}))

	b.WriteString(`</div>` + tiersCSS)

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

	// Login link (only when not logged in)
	if sess, _ := auth.TrySession(r); sess == nil {
		b.WriteString(`<p style="font-size:14px;color:#888;margin:16px 0">Already have an account? <a href="/login">Log in</a></p>`)
	}

	b.WriteString(`</div>`) // close centered column

	page := app.RenderHTMLForRequest("Pricing", "What it costs and how you pay", b.String(), r)
	w.Write([]byte(page))
}
