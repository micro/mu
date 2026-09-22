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
	b.WriteString(`<section class="plan-section section-stack"><h2>Free</h2><p>Ask Micro questions and get help with tasks.</p>`)
	if !account.PaymentsEnabled() {
		b.WriteString(`<p>No usage charges on this instance.</p></section></div>`)
		app.Respond(w, r, app.Response{Title: "Pricing", HTML: b.String()})
		return
	}
	b.WriteString(`<p>` + strconv.Itoa(account.SignupCredits) + ` signup credits, once. No card required.</p>`)
	if daily := quota.DailyCredits(); daily > 0 {
		b.WriteString(`<p>` + strconv.Itoa(daily) + ` credits per day for assistant calls and tools · resets 00:00 UTC</p>`)
		if quota.DailyPoolCredits() > 0 {
			b.WriteString(`<p>Subject to a shared daily limit.</p>`)
		}
	} else {
		b.WriteString(`<p>No daily allowance on this instance.</p>`)
	}
	if _, _, err := auth.RequireSession(r); err != nil {
		b.WriteString(`<p><a class="btn" href="/signup">Create account</a></p>`)
	}
	b.WriteString(`</section><section class="plan-section section-stack"><h2>PAYG</h2><p>Pay as you go. Top up credits when you need more.</p><p><strong>1 credit = 1 US cent.</strong> No monthly commitment or automatic top-ups.</p>`)
	if account.TopUpConfigured() {
		b.WriteString(`<p><a class="btn" href="/account/topup">Top up</a></p>`)
	}
	b.WriteString(`</section>` + account.MonthlyPricingHTML(r) + `<section id="costs" class="plan-section section-stack"><h2>Usage costs</h2><p>Assistant calls and paid tools are charged separately.</p>` + account.PricingTableHTML() + `</section>`)
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
