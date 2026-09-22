package home

import (
	"encoding/json"
	"mu/account"
	"mu/internal/app"
	"mu/internal/auth"
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

	description := "Free, PAYG and Pro"
	b.WriteString(`<section class="plan-section section-stack"><h2>Free</h2><p><strong>$0</strong></p><p>Ask questions, draft a message or make a plan.</p>`)
	if !account.PaymentsEnabled() {
		b.WriteString(`<p>No usage charges on this instance.</p></section></div>`)
		app.Respond(w, r, app.Response{Title: "Pricing", HTML: b.String()})
		return
	}
	messageCost := quota.OperationCost(quota.OpAgentRun)
	_, _, sessionErr := auth.RequireSession(r)
	if daily := quota.DailyCredits(); daily > 0 {
		if messageCost > 0 && daily >= messageCost {
			b.WriteString(`<p>Up to <strong>` + strconv.Itoa(daily/messageCost) + ` simple messages a day.</strong></p>`)
		} else {
			b.WriteString(`<p>` + strconv.Itoa(daily) + ` daily credits.</p>`)
		}
		if quota.DailyPoolCredits() > 0 {
			b.WriteString(`<p>Subject to a shared daily limit.</p>`)
		}
	} else {
		b.WriteString(`<p>No daily allowance on this instance.</p>`)
	}
	if messageCost > 0 && account.SignupCredits >= messageCost {
		b.WriteString(`<p>Plus up to ` + strconv.Itoa(account.SignupCredits/messageCost) + ` extra simple messages when you join. No card required.</p>`)
	} else {
		b.WriteString(`<p>` + strconv.Itoa(account.SignupCredits) + ` signup credits. No card required.</p>`)
	}
	if sessionErr != nil {
		b.WriteString(`<p><a class="btn" href="/signup">Create account</a></p>`)
	}
	b.WriteString(`</section><section class="plan-section section-stack"><h2>PAYG</h2><p>Pay as you go when you need more. No subscription.</p>`)
	if messageCost > 0 && messageCost <= 500 {
		b.WriteString(`<p><strong>$5</strong> covers up to <strong>` + strconv.Itoa(500/messageCost) + ` simple messages.</strong></p>`)
	}
	b.WriteString(`<p>Add credit in Account. You choose when to top up.</p>`)
	if sessionErr == nil && account.TopUpConfigured() {
		b.WriteString(`<p><a class="btn" href="/account/topup">Top up</a></p>`)
	}
	b.WriteString(`</section>` + account.MonthlyPricingHTML(r) + `<section id="costs" class="plan-section section-stack"><p>Message estimates are for assistant replies without paid tools. Briefs, searches and other paid tools use the same allowance, so you may get fewer messages.</p><details class="disclosure"><summary>Usage details</summary><p>1 credit = 1 US cent. A simple assistant reply costs ` + strconv.Itoa(messageCost) + ` credits.</p><p>Free includes ` + strconv.Itoa(account.SignupCredits) + ` signup credits and ` + strconv.Itoa(quota.DailyCredits()) + ` daily credits. Daily credits reset at 00:00 UTC and do not roll over.</p>` + account.PricingTableHTML() + `</details></section>`)
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
		"question_cost": quota.OperationCost(quota.OpAgentRun), "welcome": account.SignupCredits,
		"monthly": func() any {
			p, ok := account.MonthlyPlan()
			if ok {
				return p
			}
			return nil
		}(),
		"daily": quota.DailyCredits(), "prices": account.Pricing(), "limits": limits,
	})
}
