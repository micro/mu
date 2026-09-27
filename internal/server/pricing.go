package server

import (
	"encoding/json"
	"mu/account"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/files"
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

	description := "Start free, or choose a monthly plan for daily briefs and more credits."
	b.WriteString(`<div class="card-grid comparison-grid" aria-label="Compare plans">`)
	b.WriteString(`<section class="card plan-card"><h2>Free</h2><p class="plan-price"><strong>$0</strong></p>`)
	if !account.PaymentsEnabled() {
		b.WriteString(`<p>No usage charges on this instance.</p></section></div></div>`)
		app.Respond(w, r, app.Response{Title: "Pricing", HTML: b.String()})
		return
	}
	messageCost := quota.OperationCost(quota.OpAgentRun)
	_, _, sessionErr := auth.RequireSession(r)
	b.WriteString(`<p>Try Micro at your own pace.</p><ul class="plan-benefits"><li>` + strconv.Itoa(account.SignupCredits) + ` signup credits. No card required.</li>`)
	if daily := quota.DailyCredits(); daily > 0 {
		b.WriteString(`<li>` + strconv.Itoa(daily) + ` free credits each day.</li>`)
		if quota.DailyPoolCredits() > 0 {
			b.WriteString(`<li>Daily credits are subject to a shared limit.</li>`)
		}
	}
	b.WriteString(`<li>Weekly morning brief included.</li><li>Top up whenever you need more usage.</li></ul>`)
	if sessionErr != nil {
		b.WriteString(`<div class="form-actions"><a class="btn" href="/signup">Create account</a></div>`)
	}
	b.WriteString(`</section>` + account.MonthlyPricingHTML(r) + `</div><p class="text-muted">All plans include access to the assistant and services. Choose which scheduled events to enable. Monthly plans renew automatically; cancel any time. No additional service charge.</p><section class="plan-section section-stack"><h2>Pay as you go</h2><p><strong>1 credit = 1 US cent</strong></p><p>Top up when you need more usage. No subscription required.</p><p>Credits pay for assistant replies and paid services. Purchased credits do not expire.</p>`)
	if account.TopUpConfigured() {
		if sessionErr == nil {
			b.WriteString(`<p><a class="btn" href="/account/topup">Top up</a></p>`)
		} else {
			b.WriteString(`<p><a class="btn" href="/signup?redirect=%2Faccount%2Ftopup">Get started</a></p>`)
		}
	}
	b.WriteString(`</section><section id="costs" class="plan-section section-stack"><h2>What uses credits?</h2><p>Credits pay for assistant replies and chargeable service operations, whether you use a service directly or through Micro. A reply without paid tools costs ` + strconv.Itoa(messageCost) + ` credits. Paid operations, such as web searches, directions, sending messages and generating apps, are added to that cost.</p><p>A task can use several operations, so its total depends on what Micro needs to do. Included services remain available when you run out of credits; chargeable operations need an available allowance or balance.</p><details class="disclosure"><summary>Usage details and limits</summary><p>You can also use services directly. Keeping notes and documents, managing your calendar, storing files and browsing published news, videos and market prices do not use credits.</p><p>Files includes ` + strconv.Itoa(files.MaxOwnerBytes/(1<<20)) + ` MiB per account, up to ` + strconv.Itoa(files.MaxBytes/(1<<20)) + ` MiB per file. The same file limits apply on every plan.</p><p><a href="/services">Explore services</a></p><p>Signup credits are granted once. Daily credits reset at 00:00 UTC and do not roll over.</p>` + account.PricingTableHTML() + `</details></section>`)
	b.WriteString(`<section class="plan-section section-stack"><h2>Tools for agents</h2><p>Use Micro’s services from your own agent through MCP or the API, with the same service rates and account balance.</p><p><a href="/developers">Developer access</a></p></section></div>`)
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
	plans := []account.Plan{}
	for _, tier := range []string{"starter", "pro"} {
		if p, ok := account.SubscriptionPlan(tier); ok {
			plans = append(plans, p)
		}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"plans":    plans,
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
