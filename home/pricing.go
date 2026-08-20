package home

import (
	"net/http"
	"strings"

	"mu/account"
	"mu/internal/app"
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
// account.PricingTableHTML, which renders quota.json. An operator who changes
// the file changes this page, which is the only way a price list stays true —
// the last one drifted because four tables were maintained by hand and three of
// them had lost the most expensive operation in the product.
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

	section("The agent has no price",
		para(`Not a low one — none. Chat here, write to it from anywhere, read your
		 inbox, keep files: none of it costs credits. The agent is what this is,
		 and metering the thing you came for is a toll booth at your own front
		 door.`),
		para(`What a run costs is whatever tools it reached for, one line each on
		 your receipt. Most answers reach for nothing.`))

	section("What you pay for is fetching",
		para(`Credits buy the calls that cost somebody money: a web search, an
		 image, a text message, a place looked up. You need none until you reach
		 for one of those, and a new account starts at zero because there is
		 nothing it has to buy first.`),
		para(`And a price is what it costs to <em>go and get</em> something. If this
		 instance already has the answer — a forecast somebody else asked for in
		 the last half hour, a map tile it fetched once — you are not charged for
		 a fetch that did not happen. That is the point of a shared instance
		 rather than your own API key: the more people use it, the less any of
		 them pays.`))

	section("What things cost",
		para(`Every operation, and what it charges. This is the same table the
		 product renders on your account, from the same file.`),
		account.PricingTableHTML())

	section("Paying for what does cost",
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
