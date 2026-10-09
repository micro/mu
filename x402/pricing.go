package x402

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/x402/billing"
	x402 "mu/x402/payment"
	"net/http"
	"strings"
)

// pricingHandler presents both payment options over the same service rates.
func pricingHandler(w http.ResponseWriter, r *http.Request) {
	plans := []billing.Plan{}
	var cards strings.Builder
	for _, tier := range []string{"starter", "pro"} {
		if p, ok := billing.SubscriptionPlan(tier); ok {
			plans = append(plans, p)
			fmt.Fprintf(&cards, `<section class="card plan-card"><h2>%s</h2><p>$%.2f / month</p><p>%d credits each month for tool calls.</p><p class="text-muted">Monthly credits expire at renewal. Cancel any time.</p><a class="btn" href="/account?plan=%s#subscription">Choose %s</a></section>`, html.EscapeString(p.Name), float64(p.Cents)/100, p.Credits, p.Tier, html.EscapeString(p.Name))
		}
	}
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"plans": plans, "prices": billing.Pricing(), "credit_usd": 0.01, "x402": x402.Enabled(), "payments": billing.PaymentsEnabled()})
		return
	}
	description := "Use account credits for tool calls."
	body := `<p>Direct x402 payments are not enabled on this instance.</p>`
	if !billing.PaymentsEnabled() {
		body += `<p>No usage charges on this instance. Account permissions and usage limits still apply.</p><h2>Tools</h2>` + toolPricesHTML()
		app.Respond(w, r, app.Response{Title: "Pricing", Description: "Tool access on this instance.", HTML: body})
		return
	}
	if x402.Enabled() {
		description = "Pay as you go, or use account credits for regular tool use."
		body = `<div class="card-grid payment-options"><section class="card plan-card"><h2>Pay per call</h2><p>No subscription required.</p><p>Priced calls return an HTTP 402 payment request. Your client can pay with the supported wallet asset and retry. The response specifies the network and accepted assets.</p><a href="/tools">Explore tools</a></section>`
	} else {
		body += `<div class="card-grid payment-options">`
	}
	body += `<section class="card plan-card"><h2>Account credits</h2><p>1 credit = 1 US cent.</p><p>Use a service API token to pay from your available credits on the same MCP and HTTP endpoints. Credits can come from a subscription or a top-up.</p><a href="/account/tokens?access=services">Create a service token</a></section></div>`
	body = `<div class="section-stack">` + body
	if cards.Len() > 0 {
		body += `<h2>Monthly credits</h2><div class="card-grid payment-options">` + cards.String() + `</div>`
	}
	if billing.TopUpConfigured() || billing.CryptoConfigured() {
		body += `<div class="form-actions"><a class="btn" href="/account/topup">Top up credits</a></div>`
	}
	if x402.Enabled() {
		body += `<h2>One catalogue, two ways to pay</h2><p>Authenticated calls use available account credits. Sending an x402 payment pays for that call directly instead.</p>`
	}
	body += `<p>Account permissions and usage limits still apply; tools with no usage charge may require an account.</p><h2>Tool prices</h2>` + toolPricesHTML() + `</div>`
	app.Respond(w, r, app.Response{Title: "Pricing", Description: description, HTML: body})
}
