package home

import (
	"net/http"
	"strconv"
	"strings"

	"mu/account"
	"mu/internal/app"
	"mu/internal/quota"
)

// dailyCap is the sentence about a daily limit, or nothing when there is none.
//
// Conditional rather than a number dropped into a sentence, because
// quota.DailyLimit returns NoLimit — a negative — for an operation with no cap,
// and an operator who removes one would otherwise publish "capped at -1 a day".
// The limits are quota.json's, and quota.json is an operator's file.
func dailyCap(op string) string {
	if n := quota.DailyLimit(op); n > 0 {
		return ", capped at " + strconv.Itoa(n) + " a day"
	}
	return ""
}

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

	section("Credits",
		para(`One credit is one penny. A credit is charged when an operation costs
		 this instance money: a model call, or a third party billed per request.
		 Everything else is 0.`),
		para(`No subscription, no minimum, no expiry. Top up any amount at
		 <a href="/account">your account</a>.`),
		para(`An agent can skip the account: call without a token and a priced tool
		 answers <code>402</code> with an <a href="https://x402.org">x402</a>
		 challenge, payable in USDC on Base. See <a href="/api">the API</a>.`))

	section("The agent is free",
		para(`Chatting, your inbox, your files: no credits. You pay for the tools a
		 run reaches for, one line each on your receipt. Most answers reach for
		 none.`))

	section("Your mailbox",
		para(`Receiving costs nothing, however much arrives. Reading it costs nothing
		 — the address works in Thunderbird, Mail.app or your phone over IMAP, and
		 you can reply from there over SMTP, with an access token as the password.`),
		para(`Sending is `+pence(quota.OpMailSend)+dailyCap(quota.OpMailSend)+
			`, wherever it is going — an address outside or somebody on this
		 instance. It is the one price here that is not a cost. Nothing is spent
		 carrying a message to the account next door, but an inbox full of junk
		 from inside is the same failure as one full of junk from outside, and
		 what a loop spends on an address outside is this domain's reputation,
		 which no balance repairs.`),
		para(`Writing to yourself or to your own agent is not sending, and neither
		 is your agent answering you. Those are free and uncapped: they are what
		 you came for.`))

	section("You pay for a fetch",
		para(`A price is what it costs to go and get something. If this instance
		 already has the answer — a forecast someone asked for in the last half
		 hour, a map tile fetched once — you are not charged. The more people use
		 an instance, the less each of them pays.`))

	section("Prices", account.PricingTableHTML())

	section("Self-hosting",
		para(`One Go binary, <a href="https://github.com/micro/mu">source here</a>.
		 Every price above comes from one file in it, so an instance you run
		 charges what you set — including nothing. Callers pay you.`))

	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Pricing",
		Description: "What a credit is and what each operation costs",
		HTML:        b.String()})
}
