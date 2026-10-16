package server

import (
	"encoding/json"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/files"
	"mu/internal/quota"
	"mu/x402/billing"
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

	description := "Pay for your personal assistant. Choose a monthly plan or prepaid credits."
	if !billing.PaymentsEnabled() {
		b.WriteString(`<p>No usage charges on this self-hosted instance.</p></div>`)
		app.Respond(w, r, app.Response{Title: "Pricing", HTML: b.String()})
		return
	}
	messageCost := quota.OperationCost(quota.OpAgentRun)
	_, _, sessionErr := auth.RequireSession(r)
	b.WriteString(`<p>Try a short public chat and read the shared brief on <a href="/">the home page</a>. Personal assistant use is paid. No signup or daily credit grants.</p><div class="card-grid comparison-grid" aria-label="Compare plans">` + billing.MonthlyPricingHTML(r) + `</div><p class="text-muted">Monthly plans renew automatically; cancel any time. Choose which scheduled updates to enable.</p><section class="plan-section section-stack"><h2>Pay as you go</h2><p><strong>1 credit = 1 US cent</strong></p><p>Add prepaid credit to activate personal chat and paid tools. No subscription required. Scheduled briefs require a monthly plan.</p><p>Purchased credits do not expire.</p>`)
	if billing.TopUpConfigured() {
		if sessionErr == nil {
			b.WriteString(`<p><a class="btn" href="/account/topup">Top up</a></p>`)
		} else {
			b.WriteString(`<p><a class="btn" href="/signup?redirect=%2Faccount%2Ftopup">Get started</a></p>`)
		}
	}
	if billing.CryptoConfigured() {
		b.WriteString(`<p><a class="btn" href="/account/topup#crypto">Pay with crypto</a></p><p>Buy credits directly from your wallet with USDC on Base. 1 USDC = 100 credits.</p>`)
	}
	b.WriteString(`</section><section id="costs" class="plan-section section-stack"><h2>What uses credits?</h2><p>Credits pay for assistant replies and chargeable service operations, whether you use a service directly or through Micro. A reply without paid tools costs ` + strconv.Itoa(messageCost) + ` credits. Paid operations, such as web searches, directions, sending messages and generating apps, are added to that cost.</p><p>A task can use several operations, so its total depends on what Micro needs to do. Included services remain available when you run out of credits; chargeable operations need an available allowance or balance.</p><details class="disclosure"><summary>Usage details and limits</summary><p>You can also use services directly. Keeping notes and documents, managing your calendar, storing files and browsing published news, videos and market prices do not use credits.</p><p>Files includes ` + strconv.Itoa(files.MaxOwnerBytes/(1<<20)) + ` MiB per account, up to ` + strconv.Itoa(files.MaxBytes/(1<<20)) + ` MiB per file. The same file limits apply on every plan.</p><p><a href="/services">Explore services</a></p>` + billing.PricingTableHTML() + `</details></section>`)
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
	plans := []billing.Plan{}
	for _, tier := range []string{"starter", "pro"} {
		if p, ok := billing.SubscriptionPlan(tier); ok {
			plans = append(plans, p)
		}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"plans":    plans,
		"payments": billing.PaymentsEnabled(), "topup": billing.TopUpConfigured(),
		"question_cost": quota.OperationCost(quota.OpAgentRun), "welcome": billing.SignupCredits,
		"monthly": func() any {
			p, ok := billing.MonthlyPlan()
			if ok {
				return p
			}
			return nil
		}(),
		"daily": quota.DailyCredits(), "prices": billing.Pricing(), "limits": limits,
	})
}
