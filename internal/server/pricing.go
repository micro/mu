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

	description := "Everyday services, with flexible usage for your personal assistant."
	b.WriteString(`<section class="plan-section section-stack"><h2>Included with your account</h2><p>Mail, Events, Notes, Files, Docs, News, Markets, Video, Weather and Maps. Use services directly, or ask Micro to work across them.</p><p>Keep notes and documents, manage your calendar, store files and browse published news, videos and market prices without using credits.</p><p>Files includes ` + strconv.Itoa(files.MaxOwnerBytes/(1<<20)) + ` MiB per account, up to ` + strconv.Itoa(files.MaxBytes/(1<<20)) + ` MiB per file. The same file limits apply on every plan.</p><p><a href="/services">Explore services</a></p></section>`)
	b.WriteString(`<section class="plan-section section-stack"><h2>Free</h2><p><strong>$0</strong></p>`)
	if !account.PaymentsEnabled() {
		b.WriteString(`<p>No usage charges on this instance.</p></section></div>`)
		app.Respond(w, r, app.Response{Title: "Pricing", HTML: b.String()})
		return
	}
	messageCost := quota.OperationCost(quota.OpAgentRun)
	_, _, sessionErr := auth.RequireSession(r)
	b.WriteString(`<p>Start with the included services and try Micro with free credits.</p><p>` + strconv.Itoa(account.SignupCredits) + ` credits when you sign up. No card required.</p>`)
	if daily := quota.DailyCredits(); daily > 0 {
		b.WriteString(`<p>` + strconv.Itoa(daily) + ` free credits each day.</p>`)
		if quota.DailyPoolCredits() > 0 {
			b.WriteString(`<p>Subject to a shared daily limit.</p>`)
		}
	}
	if sessionErr != nil {
		b.WriteString(`<p><a class="btn" href="/signup">Create account</a></p>`)
	}
	b.WriteString(`</section><section class="plan-section section-stack"><h2>PAYG</h2><p>Pay as you go when you need more. No subscription.</p>`)
	b.WriteString(`<p>1 credit = 1 US cent.</p><p>Top up in Account.</p>`)
	if sessionErr == nil && account.TopUpConfigured() {
		b.WriteString(`<p><a class="btn" href="/account/topup">Top up</a></p>`)
	}
	b.WriteString(`</section>` + account.MonthlyPricingHTML(r) + `<section id="costs" class="plan-section section-stack"><h2>What uses credits?</h2><p>Credits pay for assistant replies and chargeable service operations, whether you use a service directly or through Micro. A reply without paid tools costs ` + strconv.Itoa(messageCost) + ` credits. Paid operations, such as web searches, directions, sending messages and generating apps, are added to that cost.</p><p>A task can use several operations, so its total depends on what Micro needs to do. Included services remain available when you run out of credits; chargeable operations need an available allowance or balance.</p><details class="disclosure"><summary>Usage details</summary><p>Signup credits are granted once. Daily credits reset at 00:00 UTC and do not roll over.</p>` + account.PricingTableHTML() + `</details></section>`)
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
