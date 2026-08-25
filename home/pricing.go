package home

import (
	"net/http"
	"strconv"
	"strings"

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

	// A list, for the things that are free. They were four paragraphs saying
	// "this is free" four times; what a reader wants is to scan them.
	list := func(items ...string) string {
		var b strings.Builder
		b.WriteString(`<ul class="price-free">`)
		for _, it := range items {
			b.WriteString(`<li>` + it + `</li>`)
		}
		b.WriteString(`</ul>`)
		return b.String()
	}

	// Short, because a pricing page answers one question.
	//
	// This was six sections and about four hundred words of prose. Most of it
	// was reasoning — why sending mail costs when receiving does not, what a
	// price means, what a cached answer is — and reasoning is what you write
	// when you are justifying a number rather than stating one. Three
	// paragraphs on the mailbox alone, to say "sending costs, everything else
	// about mail is free".
	//
	// The argument for each price is worth keeping and it belongs next to the
	// price in internal/quota, where somebody changing it will read it. Here,
	// the number and what it applies to.
	section("Credits",
		para(`One credit is one penny, charged when a call costs this instance
		 money — a model call, or a third party billed per request. Everything
		 else is free.`))

	section("Free",
		list(
			`The agent: chatting, your inbox, your files.`,
			`Receiving costs nothing, however much arrives. Reading it costs
			 nothing — the address works in any mail client over IMAP, and you
			 can reply from there over SMTP.`,
			`Writing to yourself or to your own agent, and its answers back.`,
			`Anything already fetched. Ask for a forecast somebody asked for half
			 an hour ago and you are not charged, so the more an instance is
			 used the less each person pays.`))

	// The prices are on the tools, not here.
	//
	// This printed the whole table, which /tools already prints against every
	// tool it lists — the same numbers in two places, which is how they come to
	// disagree. A shop does not have a pricing page; the price is on the shelf
	// edge, next to the thing. What is left here is the part that is not
	// per-item and so has no shelf to sit on.
	section("Paid",
		para(`Every tool lists what it charges — <a href="/tools">see them</a>.`),
		para(`Sending mail is `+pence(quota.OpMailSend)+dailyCap(quota.OpMailSend)+
			`, wherever it is going — an address outside, or somebody on this
			 instance. It is the one price here that is not a cost: what a loop
			 spends writing to the world is this domain's reputation, which no
			 balance repairs.`))

	section("Topping up",
		para(`Any amount, at <a href="/billing/topup">billing</a>. No subscription,
		 no minimum, no expiry.`))

	// x402 gets its own heading.
	//
	// It was the second half of a paragraph about topping up, which files the
	// most unusual thing here under the most ordinary one. Paying per call with
	// no account, no signup and no relationship is the part of this that does
	// not exist elsewhere, and somebody scanning the page for it found a
	// sentence beginning "or".
	section("Paying without an account",
		para(`Call a priced tool with no token and it answers <code>402</code>
		 with an <a href="https://x402.org">x402</a> challenge: the price, and
		 where to send it. Pay in USDC on Base and the same request succeeds.`),
		para(`No signup, no card, no account here at all — the price is named
		 before the work is done, and nothing is owed after it. Point an agent
		 at <a href="/mcp">/mcp</a>; <a href="/tools">every tool</a> lists what
		 it charges.`))

	section("Self-hosting",
		para(`One Go binary, <a href="https://github.com/micro/mu">source here</a>.
		 Every price above comes from one file in it, so an instance you run
		 charges what you set — including nothing. Callers pay you.`))

	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Pricing",
		Description: "What a credit is and what each operation costs",
		HTML:        b.String()})
}
