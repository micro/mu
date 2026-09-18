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
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{
			Title:       "Pricing",
			Description: "This instance does not charge for anything.",
			HTML:        b.String(),
		})
		return
	}

	description := "Pay for what you use, with no subscription."
	b.WriteString(`<div class="card"><h3>Everyday use</h3>`)
	if daily := quota.DailyCredits(); daily > 0 {
		description = "A free daily allowance, with optional credit for more use."
		b.WriteString(`<p>Every account includes ` + strconv.Itoa(daily) + ` credits each day. No payment or card is needed. The allowance renews at 00:00 UTC and is used before any credit you add.</p>` +
			`<p>Unused daily credit does not carry over. Your conversations and saved items remain available when you reach the allowance.</p>`)
		if quota.DailyPoolCredits() > 0 {
			b.WriteString(`<p>Free use also shares an instance-wide daily budget. If that is used up, it renews at 00:00 UTC; added credit remains available.</p>`)
		}
	} else {
		b.WriteString(`<p>A new account includes ` + creditsInWords() + `. No card is needed to start.</p>`)
	}
	b.WriteString(`</div>`)
	if account.TopUpConfigured() {
		b.WriteString(`<div class="card"><h3>If you need more</h3><p>You can add credit for more use. One credit is one US cent. There is no subscription, and an idle account is charged nothing.</p>` +
			`<div class="form-actions"><a href="/signup" class="btn">Create an account</a><a href="/account/topup">Add credit</a></div></div>`)
	}
	b.WriteString(`<details class="card"><summary>Usage costs</summary><p>An assistant reply uses ` +
		strconv.Itoa(quota.OperationCost(quota.OpAgentRun)) + ` credits. Paid tools and message delivery may use additional credit. These come from your daily allowance first, then your balance. Messaging limits still apply.</p>` +
		`<p>Reading your conversations, mail and saved items is free.</p>` + account.PricingTableHTML() + `</details>`)
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Pricing", Description: description, HTML: b.String()})
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
