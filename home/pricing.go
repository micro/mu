package home

import (
	"net/http"
	"strconv"
	"strings"

	"mu/account"
	"mu/internal/app"
	"mu/internal/quota"
)

// PricingHandler serves /pricing.
//
// This page has been deleted before, and the reason it was deleted still
// stands: it was three attempts at a plan chooser — a cost table, then three
// columns of plans, then plans without columns — and every version came apart
// on the same fact. A credit is a penny and every operation costs what
// quota.json says, whoever is asking. There is nothing to choose, so a page
// asking somebody to choose was inventing a decision to make this feel like a
// product with tiers.
//
// What is back is not that. It is the price list, which is a different thing:
// somebody deciding whether to use this needs to know what it costs before they
// have an account, and until now the only answer was /tools — a catalogue of a
// hundred-odd entries, each carrying its own price, which answers "what does
// this one call cost" and never "what is this going to cost me". A signed-out
// visitor had nowhere at all to read the terms.
//
// No plans on it, and none should appear. If a tier is ever added, it belongs
// here as a fact and not as three columns with a recommended one in the middle.
//
// Every number is read rather than written: the table comes from
// account.PricingTableHTML, which renders quota.json, and the allowance is
// quota.DailyQuota. An operator who changes the file changes this page, which
// is the only way a price list stays true — the last one drifted because four
// tables were maintained by hand and three of them had lost the most expensive
// operation in the product.
func PricingHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(app.Column())

	section := func(title string, body ...string) {
		b.WriteString(`<div class="card"><h3>` + title + `</h3>`)
		for _, p := range body {
			b.WriteString(p)
		}
		b.WriteString(`</div>`)
	}

	para := func(s string) string { return `<p>` + s + `</p>` }

	section("What a credit is",
		para(`One credit is one penny. Nothing costs a fraction of one and nothing
		 is priced in a second unit, so the number beside an operation is what it
		 costs and the balance on your account is money.`),
		para(`A credit is charged when an operation costs this instance something to
		 run: a model call, or a paid third party behind it. Anything that only
		 touches storage here is free, and free means free rather than
		 included-up-to-a-limit.`))

	if quota.DailyQuota > 0 {
		section("Free every day",
			para(`Every account gets <strong>`+strconv.Itoa(quota.DailyQuota)+
				` credits a day</strong>, renewed daily, with no card and nothing to
			 cancel. It is enough to use the agent properly rather than enough to
			 look at it, and it is what most people will ever need.`),
			para(`This is an allowance, not a tier: the operator of an instance sets
			 the number, and running your own means setting it yourself.`))
	}

	section("What things cost",
		para(`Every operation, and what it charges. This is the same table the
		 product renders on your account, from the same file.`),
		account.PricingTableHTML())

	section("Paying past the allowance",
		para(`Top up in whatever amount suits you from <a href="/account">your
		 account</a> and it is drawn down as you go. There is no subscription, no
		 minimum, and credits do not expire.`),
		para(`An agent calling the tools directly can pay per request instead, with
		 no account here at all — see <a href="/tools">the tools</a> for how, and
		 <a href="/api">the API reference</a> for the endpoints.`))

	section("Running your own",
		para(`Mu is one Go binary and the source is
		 <a href="https://github.com/micro/mu">public</a>. Every price on this page
		 comes from a single file in it, so an instance you run charges what you
		 decide — including nothing. Anyone paying to call your tools pays you.`))

	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Pricing",
		Description: "What a credit is, what each operation costs, and what is free",
		HTML:        b.String()})
}
