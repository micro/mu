package home

import (
	"encoding/json"
	"mu/account"
	"mu/internal/app"
	"mu/internal/quota"
	"net/http"
	"strconv"
	"strings"
)

func PricingHandler(w http.ResponseWriter, r *http.Request) {
	if app.WantsJSON(r) {
		pricingHandlerJSON(w, r)
		return
	}
	var b strings.Builder
	b.WriteString(app.Column())

	// An instance nobody can pay charges nobody. Saying that is the whole page:
	// the operator of a box with no Stripe keys has not decided to be cheap,
	// they have decided not to charge, and printing a table of prices that
	// cannot be applied invites somebody to worry about a bill that does not
	// exist.
	if !account.PaymentsEnabled() {
		b.WriteString(`<div class="card"><h3>What this costs</h3>` +
			`<p>Nothing. This instance is not set up to take payments, so nothing on it ` +
			`is metered — whoever runs it is paying for the models and the searches it ` +
			`makes.</p>` +
			`<p class="text-sm"><a href="/install">Run your own &rarr;</a> · ` +
			`<a href="/about">What this is</a></p></div>`)
		app.Respond(w, r, app.Response{
			Title:       "Pricing",
			Description: "This instance does not charge for anything.",
			HTML:        b.String(),
		})
		return
	}

	// The headline number in the prose, and read from the price list rather
	// than written into the sentence.
	//
	// The table below is sorted cheapest first, so the one price nearly
	// everybody is here to know sits somewhere in the middle of twenty rows
	// looking like the cost of a pollen forecast. Saying it in the paragraph is
	// the answer; hardcoding it there would be a lie on the day an operator
	// changes it, and this file is served by every instance.
	b.WriteString(`<div class="card"><h3>What this costs</h3>` +
		`<p>A question to the agent is ` + strconv.Itoa(quota.OperationCost(quota.OpAgentRun)) +
		`¢. This instance charges for what it has to buy from somebody else: a question is a ` +
		`model call, a search is a search company's, a text message is a carrier's. ` +
		`Anything that only touches this server — reading the news, your mail, your ` +
		`notes, the archive — is free, because serving it costs nothing.</p>` +
		account.PricingTableHTML() +
		`</div>`)

	// The two facts somebody actually needs, and they are about the money
	// rather than about any one operation: what you get for nothing, and what a
	// credit is. The table above answers neither.
	start := `<div class="card"><h3>Starting</h3>` +
		`<p>A new account gets ` + creditsInWords() + ` to find out whether this is ` +
		`useful — about thirty questions. Nothing is asked for up front and there is ` +
		`no card to leave.</p>`
	if account.TopUpConfigured() {
		start += `<p>After that you top up: $5, $10, $25 or $50, or any amount you type. ` +
			`A credit is a cent, and it is spent on what you use rather than on a plan — ` +
			`there is no subscription, and an account that sits idle is charged nothing.</p>` +
			`<p class="text-sm"><a href="/signup" class="btn">Sign up</a> ` +
			`<a href="/account/topup" class="btn btn-secondary">Top up</a></p>`
	} else {
		// Metered but with no card route configured — x402 only. Real, and the
		// page must not offer a top-up form that is not there.
		start += `<p>This instance takes payment over x402 rather than by card, so an ` +
			`agent pays per request from its own wallet.</p>`
	}
	start += `</div>`
	b.WriteString(start)

	b.WriteString(`<div class="card"><h3>Running it yourself</h3>` +
		`<p>The software is the same either way, and an instance you run has no meter ` +
		`in it: you hold the API keys and pay the providers directly.</p>` +
		`<p class="text-sm"><a href="/install">How to run it</a> · ` +
		`<a href="/about">What this is</a> · ` +
		`<a href="https://github.com/micro/mu">The source</a></p></div>`)

	app.Respond(w, r, app.Response{
		Title:       "Pricing",
		Description: "What this instance costs: a dollar of credit to start, then a credit is a cent.",
		HTML:        b.String(),
	})
}

func pricingHandlerJSON(w http.ResponseWriter, r *http.Request) {
	type limit struct {
		Label string `json:"label"`
		Limit int    `json:"limit"`
	}
	limits := []limit{}
	for _, p := range quota.Prices() {
		if n := quota.DailyLimit(p.Op); n != quota.NoLimit {
			limits = append(limits, limit{p.Label, n})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	json.NewEncoder(w).Encode(map[string]any{
		"payments": account.PaymentsEnabled(), "topup": account.TopUpConfigured(),
		"question_cost": quota.OperationCost(quota.OpAgentRun), "welcome": account.WelcomeCredits,
		"daily": quota.DailyCredits(), "prices": account.Pricing(), "limits": limits,
	})
}

func creditsInWords() string {
	c := account.WelcomeCredits
	if c%100 == 0 {
		return "$" + strconv.Itoa(c/100) + " of credit"
	}
	return strconv.Itoa(c) + " credits"
}
