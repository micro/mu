package wallet

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"mu/internal/quota"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

var (
	processedSessions = make(map[string]bool)
)

func stripeSecret() string  { return settings.Get("STRIPE_SECRET_KEY") }
func stripePortal() string  { return strings.TrimSpace(settings.Get("STRIPE_PORTAL_URL")) }
func stripePublic() string  { return settings.Get("STRIPE_PUBLISHABLE_KEY") }
func stripeWebhook() string { return settings.Get("STRIPE_WEBHOOK_SECRET") }

func StripeEnabled() bool {
	return stripeSecret() != "" && stripePublic() != ""
}

// Subscription plans — monthly credit bundles via Stripe.
//
// This is the list, and /pricing renders from it rather than keeping its own.
// They disagreed: /wallet sold Starter at £5 and Pro at £10 while /pricing
// advertised a free tier, Pro at £20 and Premium at £100, so what a visitor was
// promised and what they could actually buy were different pages nobody had
// read together. A plan is a thing you can purchase; the only place that can be
// right about it is the place that takes the money.
type SubscriptionPlan struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Price   int    `json:"price"`   // Monthly price in pence
	Credits int    `json:"credits"` // Credits granted each month
	Label   string `json:"label"`

	// What the plan sells beyond the credits. Credits are 1p on every plan, so
	// these are the difference between them — and they are numbers rather than
	// words because something has to read them.
	//
	// Agents is how many the account may keep. It is on the card.
	Agents int `json:"agents"`

	// PostsPerHour is the write rate limit, and it is deliberately not on the
	// card. A plan raises it, because somebody paying monthly is more
	// accountable than somebody who signed up a minute ago, but it is an abuse
	// control on writing to the social side rather than something anybody
	// buys. This is an MCP server: what a buyer means by a rate limit is how
	// hard their agent may call tools, and that is not this number.
	//
	// A limit is not a feature. Selling one is how a card ends up answering a
	// question nobody asked.
	PostsPerHour int `json:"posts_per_hour"`

	// Limits is how much of an operation a day this plan allows, by quota
	// operation id. Absent means quota.json's own number stands.
	//
	// This is the third thing a plan sells and the honest one: the outbound
	// operations — an email, a text, a WhatsApp conversation — are the only
	// ones whose cost to us is not covered by the credits, because what a bad
	// month spends is a domain's or a number's reputation and no balance
	// repairs that. Selling volume on them is how every provider of the same
	// thing sells, which means nobody has to have it explained.
	//
	// What is deliberately not here is switching them off. A card on file is
	// what makes abuse expensive, and every subscriber has one — so gating
	// sending behind a *tier* gates on something every paying account already
	// has, and gates out the smallest customer for no safety gained.
	Limits map[string]int `json:"limits,omitempty"`

	// Concurrency is how many tool calls this account may have running at once.
	//
	// This is the rate limit an MCP buyer actually means. The cards used to sell
	// "higher rate limits" against POST_LIMIT_PER_HOUR — the abuse control on
	// writing to the social timeline — which answers a question nobody asked.
	// What an agent hits is this: fan out across twenty tools and the twenty-first
	// waits. It is also the axis that costs us, because concurrent calls are
	// concurrent provider calls and concurrent memory.
	//
	// Enforced in internal/service/gateway.go, which every call already passes
	// through, so there is one place it can be true.
	Concurrency int `json:"concurrency"`

	Features []string `json:"features"` // extra lines for the card, beyond the above
	Featured bool     `json:"featured"` // the one the pricing page highlights
}

// There is no free plan.
//
// Every priced call here spends somebody's money — Atlas for inference, Brave
// for search, Google for places and routes, Twilio for a text — so a free tier
// loses money in proportion to how much it is used, and the accounts that use
// it most cost the most. The free option is self-hosting, which is real: AGPL,
// your own provider keys, and an instance with no Stripe and no x402 cannot
// charge anybody.
//
// The old plans sold credits at par — £5 bought 500 credits, which is £5 of
// credits — so the subscription sold nothing a top-up did not, except £10 for
// 1,200, a 20% discount on the one thing with a marginal cost. A credit is 1p
// on every plan now, and what a plan sells is scale.
var SubscriptionPlans = []SubscriptionPlan{
	{
		ID: "pro", Name: "Pro", Price: 2000, Credits: 2000,
		Label:        "£20/month — 2,000 credits",
		Agents:       5,
		PostsPerHour: 300,
		Limits: map[string]int{
			quota.OpExternalEmail: 100,
			quota.OpSMSSend:       25,
			quota.OpWhatsAppSend:  50,
		},
		Concurrency: 8,
		Features:    []string{"Everything in pay-as-go"},
		Featured:    true,
	},
	{
		ID: "scale", Name: "Scale", Price: 10000, Credits: 10000,
		Label:        "£100/month — 10,000 credits",
		Agents:       25,
		PostsPerHour: 1200,
		Limits: map[string]int{
			quota.OpExternalEmail: 1000,
			quota.OpSMSSend:       100,
			quota.OpWhatsAppSend:  200,
		},
		Concurrency: 32,
		Features:    []string{"Everything in Pro"},
	},
}

// noPlan is pay as you go, and it is not a free tier.
//
// You still pay — top up any amount, a credit is a penny — and you get the
// entire read catalogue, which is the pitch unaltered: news, weather, markets,
// places, routes, search, and everything else that answers a question. Nothing
// anybody comes here for is behind a wall.
//
// What it does not include is volume on the three operations that leave the
// building, which take quota.json's floor rather than a plan's. That is a wall
// against abuse rather than against a customer: what an account sends under our
// domain and our number is the one cost a balance cannot make whole.
//
// It replaced a third paid tier at £10. With only credits, agents and send
// volume to sell, a third column had to invent a difference between itself and
// the one above — and the column that was actually missing was the one for
// somebody who wants the tools and does not want a subscription.
var noPlan = SubscriptionPlan{
	ID: "", Name: "Pay as you go", Agents: 1, PostsPerHour: 60, Concurrency: 2,
}

// LimitFor is what this plan allows of an operation, and whether it says
// anything. A plan that is silent leaves quota.json's number in place.
func (p SubscriptionPlan) LimitFor(operation string) (int, bool) {
	if p.Limits == nil {
		return 0, false
	}
	n, ok := p.Limits[operation]
	return n, ok
}

// Plans is the catalogue, for pages that render it.
func Plans() []SubscriptionPlan { return SubscriptionPlans }

// setPlan records which plan an account is on, after a payment for it.
//
// Empty is ignored rather than written: a one-off top-up goes through the same
// webhook branch and carries no plan id, and clearing somebody's plan because
// they bought £5 of credits would take away what they are paying monthly for.
func setPlan(userID, planID string) {
	if userID == "" || planID == "" {
		return
	}
	acc, err := auth.GetAccount(userID)
	if err != nil {
		app.Log("stripe", "cannot set plan %s: no account %s", planID, userID)
		return
	}
	if acc.Plan == planID {
		return
	}
	acc.Plan = planID
	if err := auth.UpdateAccount(acc); err != nil {
		app.Log("stripe", "failed to record plan %s for %s: %v", planID, userID, err)
		return
	}
	app.Log("stripe", "%s is now on the %s plan", userID, planID)
}

// setCustomer records who Stripe thinks an account is.
//
// Written once and then left alone. Stripe reuses a customer across purchases,
// so the first one is the one that holds the cards and the subscriptions;
// overwriting it on a later checkout that happened to mint a second would
// point the billing portal at the emptier of the two.
func setCustomer(userID, customerID string) {
	if userID == "" || !strings.HasPrefix(customerID, "cus_") {
		return
	}
	acc, err := auth.GetAccount(userID)
	if err != nil || acc.Customer == customerID {
		return
	}
	if acc.Customer != "" {
		return
	}
	acc.Customer = customerID
	if err := auth.UpdateAccount(acc); err != nil {
		app.Log("stripe", "failed to record customer %s for %s: %v", customerID, userID, err)
		return
	}
	app.Log("stripe", "%s is stripe customer %s", userID, customerID)
}

// CustomerFor is the Stripe customer for an account, finding it by email when
// it was never recorded.
//
// The fallback is not tidiness. Anybody who subscribed before the webhook
// started keeping this has no customer id, and without one they cannot reach
// the portal — so the people most in need of a cancel button would be the only
// ones without it. Stripe indexes customers by email, and an account with a
// verified address is exactly the case where that lookup is safe: it matches on
// something the account has proved is its own.
func CustomerFor(userID string) (string, error) {
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return "", fmt.Errorf("account not found")
	}
	if acc.Customer != "" {
		return acc.Customer, nil
	}
	if !acc.EmailVerified || acc.Email == "" {
		return "", fmt.Errorf("this account has no billing record here yet")
	}
	id, err := customerByEmail(acc.Email)
	if err != nil || id == "" {
		return "", fmt.Errorf("no billing record found for %s", acc.Email)
	}
	setCustomer(userID, id)
	return id, nil
}

// customerByEmail asks Stripe for the customer holding an address.
func customerByEmail(email string) (string, error) {
	req, err := http.NewRequest("GET",
		"https://api.stripe.com/v1/customers?limit=1&email="+neturl.QueryEscape(email), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+stripeSecret())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", nil
	}
	return out.Data[0].ID, nil
}

// CreatePortalSession opens Stripe's own billing page for an account.
//
// The portal rather than a cancel button of our own, and it is not laziness:
// cancelling is one of four things somebody needs here, alongside changing
// plan, replacing a card and finding an invoice. Stripe hosts all four on a
// page that already holds the card details, so building one of them ourselves
// would leave three that still had nowhere to happen.
//
// It has to be switched on in the Stripe dashboard — Settings, Billing,
// Customer portal — and the error says so, because an operator who has not done
// it will otherwise read this as our bug.
// A session is minted per customer, which lands somebody straight in their own
// billing page. STRIPE_PORTAL_URL is the fallback — Stripe's shareable portal
// link, which works with no customer id at all because it asks the visitor for
// their email and sends them a magic link. Worse, and the difference matters
// for exactly one group: anybody who subscribed before the customer id was
// being recorded has none, and a cancel button they cannot use is no better
// than the one they did not have.
func CreatePortalSession(userID, returnURL string) (string, error) {
	if !StripeEnabled() {
		if link := stripePortal(); link != "" {
			return link, nil
		}
		return "", fmt.Errorf("stripe not configured")
	}
	customer, err := CustomerFor(userID)
	if err != nil {
		if link := stripePortal(); link != "" {
			app.Log("stripe", "no customer for %s, using the shared portal link: %v", userID, err)
			return link, nil
		}
		return "", err
	}

	form := neturl.Values{}
	form.Set("customer", customer)
	form.Set("return_url", returnURL)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/billing_portal/sessions",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+stripeSecret())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out struct {
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.URL == "" {
		if link := stripePortal(); link != "" {
			app.Log("stripe", "portal session refused, using the shared link: %s", out.Error.Message)
			return link, nil
		}
		why := strings.TrimSpace(out.Error.Message)
		if why == "" {
			why = "stripe returned no portal url"
		}
		return "", fmt.Errorf("%s — the customer portal may need turning on in the "+
			"Stripe dashboard, under Settings, Billing, Customer portal", why)
	}
	return out.URL, nil
}

// PortalAvailable reports whether there is any way to reach a billing portal,
// so a page does not draw a button that cannot work.
func PortalAvailable() bool { return StripeEnabled() || stripePortal() != "" }

// priceIDFor is the Stripe Price this plan is sold as, when an operator has
// made one: STRIPE_PRICE_PRO, STRIPE_PRICE_SCALE.
//
// Without it the checkout sends price_data — an inline amount and a product
// name — and Stripe mints a throwaway product and price for each purchase.
// That works for taking money and fails for everything after it: the customer
// portal can only offer a switch between Prices you have listed in its
// configuration, and an ad-hoc price cannot be listed. So an operator who has
// not made real Prices has a dashboard full of one-off products and no way to
// turn plan switching on.
//
// Optional rather than required, because an instance that only ever takes
// payments does not need them, and demanding Stripe dashboard setup before the
// first pound can be charged is a worse first run.
// Written out per plan rather than built from the id. A name assembled at run
// time is invisible to anything that scans for what the code reads, and this
// repo has a test that holds every setting against the install guide — so a
// composed name documents itself nowhere and warns nobody. Adding a plan means
// adding a line here, and that test is what says so.
func priceIDFor(planID string) string {
	switch planID {
	case "pro":
		return strings.TrimSpace(settings.Get("STRIPE_PRICE_PRO"))
	case "scale":
		return strings.TrimSpace(settings.Get("STRIPE_PRICE_SCALE"))
	}
	return ""
}

// CheckPlans asks Stripe whether what it sells matches what this instance
// advertises, and returns what does not line up.
//
// Two systems hold the price and nothing compared them. The catalogue here says
// Pro is £20 a month; the Price id in the environment points at whatever an
// operator selected in a dashboard, and a mistake there charges the wrong
// amount forever without a single error — the checkout succeeds, the webhook
// fires, the account gets its plan, and only the card statement disagrees.
//
// Read-only, and it names what to fix rather than only that something is wrong.
func CheckPlans() []string {
	if !StripeEnabled() {
		return []string{"Stripe is not configured, so no plan can be bought"}
	}

	var problems []string
	for _, p := range SubscriptionPlans {
		id := priceIDFor(p.ID)
		if id == "" {
			problems = append(problems, fmt.Sprintf(
				"%s has no Stripe Price (STRIPE_PRICE_%s). It can still be bought — the "+
					"checkout sends an inline amount — but the customer portal cannot offer a "+
					"switch to it, because it can only list Prices that exist",
				p.Name, strings.ToUpper(p.ID)))
			continue
		}
		price, err := fetchPrice(id)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s could not be read from Stripe — %v",
				p.Name, id, err))
			continue
		}
		if price.UnitAmount != p.Price {
			problems = append(problems, fmt.Sprintf(
				"%s is advertised at %s here and sells at %s in Stripe (%s). The card is "+
					"charged the Stripe amount",
				p.Name, pounds(p.Price), pounds(price.UnitAmount), id))
		}
		if !strings.EqualFold(price.Currency, "gbp") {
			problems = append(problems, fmt.Sprintf("%s sells in %s, and every price here is in pounds",
				p.Name, strings.ToUpper(price.Currency)))
		}
		if price.Recurring.Interval != "month" {
			every := price.Recurring.Interval
			if every == "" {
				every = "once, not on a subscription"
			}
			problems = append(problems, fmt.Sprintf("%s renews %s in Stripe, and is sold here as monthly",
				p.Name, every))
		}
		if !price.Active {
			problems = append(problems, fmt.Sprintf("%s points at an archived Price (%s), which cannot be bought",
				p.Name, id))
		}
	}
	if stripePortal() == "" {
		problems = append(problems, "No STRIPE_PORTAL_URL. Anybody who subscribed before "+
			"their Stripe customer id was being recorded has no way to reach the billing "+
			"portal, and so no way to cancel")
	}
	return problems
}

type stripePrice struct {
	UnitAmount int    `json:"unit_amount"`
	Currency   string `json:"currency"`
	Active     bool   `json:"active"`
	Recurring  struct {
		Interval string `json:"interval"`
	} `json:"recurring"`
}

func fetchPrice(id string) (stripePrice, error) {
	var out stripePrice
	req, err := http.NewRequest("GET", "https://api.stripe.com/v1/prices/"+neturl.PathEscape(id), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+stripeSecret())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	var body struct {
		stripePrice
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return out, err
	}
	if body.Error.Message != "" {
		return out, fmt.Errorf("%s", body.Error.Message)
	}
	return body.stripePrice, nil
}

// pounds renders pence for an operator reading a mismatch.
func pounds(pence int) string {
	if pence%100 == 0 {
		return fmt.Sprintf("£%d", pence/100)
	}
	return fmt.Sprintf("£%.2f", float64(pence)/100)
}

// planForPriceID is which plan is sold as this Stripe Price, or "".
func planForPriceID(priceID string) string {
	if strings.TrimSpace(priceID) == "" {
		return ""
	}
	for _, p := range SubscriptionPlans {
		if id := priceIDFor(p.ID); id != "" && id == priceID {
			return p.ID
		}
	}
	return ""
}

// planForPrice is which plan costs this much a month, or "" for none of them.
//
// Price rather than a stored id, because a plan can be changed in Stripe's own
// portal and the subscription's metadata does not follow: it is written when
// the subscription is created and Stripe has no reason to rewrite it when the
// price changes. So the metadata says what somebody signed up for and the price
// says what they are paying now, and only one of those is worth acting on.
//
// It works because the prices are distinct, which is a fact about the
// catalogue rather than a guarantee. Two plans at the same price would be
// ambiguous here — and would also be two plans nobody could tell apart on the
// pricing page, so it is the kind of thing that breaks visibly first.
func planForPrice(pence int) string {
	for _, p := range SubscriptionPlans {
		if p.Price == pence {
			return p.ID
		}
	}
	return ""
}

// clearPlan puts an account back on no plan, when its subscription ends.
func clearPlan(userID string) {
	if userID == "" {
		return
	}
	acc, err := auth.GetAccount(userID)
	if err != nil || acc.Plan == "" {
		return
	}
	was := acc.Plan
	acc.Plan = ""
	if err := auth.UpdateAccount(acc); err != nil {
		app.Log("stripe", "failed to end plan %s for %s: %v", was, userID, err)
		return
	}
	app.Log("stripe", "%s is no longer on the %s plan", userID, was)
}

// PlanByID is what an account on this plan is allowed. An unknown or empty id
// gets noPlan rather than nothing, so a caller never has to handle a missing
// plan and a subscription Stripe knows about but this build does not cannot
// leave somebody with a limit of zero.
func PlanByID(id string) SubscriptionPlan {
	for _, p := range SubscriptionPlans {
		if p.ID == id {
			return p
		}
	}
	return noPlan
}

// CreateSubscriptionSession creates a Stripe Checkout Session for a
// recurring subscription. Credits are granted on each successful payment
// via the invoice.payment_succeeded webhook.
func CreateSubscriptionSession(userID, planID, successURL, cancelURL string) (string, error) {
	if !StripeEnabled() {
		return "", fmt.Errorf("stripe not configured")
	}

	var plan *SubscriptionPlan
	for i := range SubscriptionPlans {
		if SubscriptionPlans[i].ID == planID {
			plan = &SubscriptionPlans[i]
			break
		}
	}
	if plan == nil {
		return "", fmt.Errorf("unknown plan: %s", planID)
	}

	// A real Price when the operator has made one, an inline amount otherwise.
	//
	// Same charge either way; the difference is everything that happens after.
	// A price_data purchase creates a throwaway product per checkout, and the
	// customer portal can only offer a switch between Prices listed in its
	// configuration — so plan switching cannot be turned on at all until these
	// exist. See priceIDFor.
	item := map[string]interface{}{"quantity": 1}
	if priceID := priceIDFor(plan.ID); priceID != "" {
		item["price"] = priceID
	} else {
		item["price_data"] = map[string]interface{}{
			"currency": "gbp",
			"recurring": map[string]interface{}{
				"interval": "month",
			},
			"unit_amount": plan.Price,
			"product_data": map[string]interface{}{
				"name":        plan.Name + " Plan",
				"description": fmt.Sprintf("%d credits/month", plan.Credits),
			},
		}
	}

	data := map[string]interface{}{
		"mode":        "subscription",
		"success_url": successURL,
		"cancel_url":  cancelURL,
		"line_items":  []map[string]interface{}{item},
		"metadata": map[string]string{
			"user_id": userID,
			"plan_id": plan.ID,
			"credits": fmt.Sprintf("%d", plan.Credits),
		},
		"subscription_data": map[string]interface{}{
			"metadata": map[string]string{
				"user_id": userID,
				"plan_id": plan.ID,
				"credits": fmt.Sprintf("%d", plan.Credits),
			},
		},
	}

	formData := jsonToForm(data)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+stripeSecret())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error.Message != "" {
		return "", fmt.Errorf("stripe: %s", result.Error.Message)
	}
	return result.URL, nil
}

// StripeTopupTier represents a Stripe topup option
type StripeTopupTier struct {
	Amount  int    `json:"amount"`  // Price in pence (e.g., 500 = £5)
	Credits int    `json:"credits"` // Credits received (equals Amount, flat rate)
	Label   string `json:"label"`   // Display label
}

// StripeTopupTiers - preset topup amounts for Stripe
var StripeTopupTiers = []StripeTopupTier{
	{Amount: 500, Credits: 500, Label: "£5"},
	{Amount: 1000, Credits: 1000, Label: "£10"},
	{Amount: 2500, Credits: 2500, Label: "£25"},
	{Amount: 5000, Credits: 5000, Label: "£50"},
}

// CreateCheckoutSession creates a Stripe Checkout Session for topup
func CreateCheckoutSession(userID string, amount int, successURL, cancelURL string) (string, error) {
	if !StripeEnabled() {
		return "", fmt.Errorf("stripe not configured")
	}

	if amount < 100 {
		return "", fmt.Errorf("minimum top-up is £1")
	}

	// Flat rate: 1 pence = 1 credit
	credits := amount
	label := fmt.Sprintf("£%d", amount/100)

	// Build request body
	data := map[string]interface{}{
		"mode":        "payment",
		"success_url": successURL,
		"cancel_url":  cancelURL,
		"line_items": []map[string]interface{}{
			{
				"price_data": map[string]interface{}{
					"currency":    "gbp",
					"unit_amount": amount,
					"product_data": map[string]interface{}{
						"name":        fmt.Sprintf("%d Credits", credits),
						"description": fmt.Sprintf("Mu credits top-up (%s)", label),
					},
				},
				"quantity": 1,
			},
		},
		"metadata": map[string]string{
			"user_id": userID,
			"credits": fmt.Sprintf("%d", credits),
		},
	}

	// Convert to form-urlencoded (Stripe API requires this)
	formData := jsonToForm(data)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(stripeSecret(), "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		app.Log("stripe", "checkout session error: %s", string(body))
		return "", fmt.Errorf("stripe error: %s", resp.Status)
	}

	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	app.Log("stripe", "created checkout session %s for user %s, %d credits", result.ID, userID, credits)
	return result.URL, nil
}

// jsonToForm converts nested JSON to Stripe's form-urlencoded format
func jsonToForm(data map[string]interface{}) string {
	var parts []string
	encodeValue("", data, &parts)
	return strings.Join(parts, "&")
}

func encodeValue(prefix string, v interface{}, parts *[]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, v := range val {
			newPrefix := k
			if prefix != "" {
				newPrefix = prefix + "[" + k + "]"
			}
			encodeValue(newPrefix, v, parts)
		}
	case map[string]string:
		for k, v := range val {
			newPrefix := k
			if prefix != "" {
				newPrefix = prefix + "[" + k + "]"
			}
			*parts = append(*parts, fmt.Sprintf("%s=%s", newPrefix, urlEncode(v)))
		}
	case []map[string]interface{}:
		for i, item := range val {
			newPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			encodeValue(newPrefix, item, parts)
		}
	case string:
		*parts = append(*parts, fmt.Sprintf("%s=%s", prefix, urlEncode(val)))
	case int:
		*parts = append(*parts, fmt.Sprintf("%s=%d", prefix, val))
	case int64:
		*parts = append(*parts, fmt.Sprintf("%s=%d", prefix, val))
	case float64:
		*parts = append(*parts, fmt.Sprintf("%s=%v", prefix, val))
	}
}

func urlEncode(s string) string {
	// Simple URL encoding for common characters
	s = strings.ReplaceAll(s, " ", "%20")
	s = strings.ReplaceAll(s, "&", "%26")
	s = strings.ReplaceAll(s, "=", "%3D")
	s = strings.ReplaceAll(s, "+", "%2B")
	return s
}

// HandleStripeWebhook processes Stripe webhook events
func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify webhook signature — REQUIRED for security
	if stripeWebhook() == "" {
		app.Log("stripe", "CRITICAL: STRIPE_WEBHOOK_SECRET not configured, rejecting webhook")
		http.Error(w, "webhook not configured", http.StatusServiceUnavailable)
		return
	}
	sig := r.Header.Get("Stripe-Signature")
	if !verifyStripeSignature(body, sig, stripeWebhook()) {
		app.Log("stripe", "webhook signature verification failed")
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	// Parse event
	var event struct {
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		app.Log("stripe", "webhook parse error: %v", err)
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}

	app.Log("stripe", "webhook received: %s", event.Type)

	// Handle checkout.session.completed
	if event.Type == "checkout.session.completed" {
		var session struct {
			ID            string `json:"id"`
			PaymentStatus string `json:"payment_status"`
			// Customer is who Stripe thinks this is, and it is the only handle
			// on a subscription after it exists. Without it there is no way to
			// open a billing portal, so there is no way to cancel — which is
			// what this account had: a monthly charge with no customer-side
			// exit but a failed card or a chargeback.
			Customer string `json:"customer"`
			Metadata struct {
				UserID  string `json:"user_id"`
				Credits string `json:"credits"`
				PlanID  string `json:"plan_id"`
			} `json:"metadata"`
			AmountTotal int `json:"amount_total"`
		}
		if err := json.Unmarshal(event.Data.Object, &session); err != nil {
			app.Log("stripe", "session parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}

		if session.PaymentStatus == "paid" {
			// Check for duplicate processing
			mutex.Lock()
			if processedSessions[session.ID] {
				mutex.Unlock()
				app.Log("stripe", "session %s already processed, skipping", session.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			processedSessions[session.ID] = true
			mutex.Unlock()

			userID := session.Metadata.UserID
			var credits int
			fmt.Sscanf(session.Metadata.Credits, "%d", &credits)

			if userID != "" && credits > 0 {
				err := AddCredits(userID, credits, quota.OpTopup, map[string]interface{}{
					"source":     "stripe",
					"session_id": session.ID,
					"amount":     session.AmountTotal,
				})
				if err != nil {
					app.Log("stripe", "failed to credit user %s: %v", userID, err)
				} else {
					app.Log("stripe", "credited %d to user %s (session %s)", credits, userID, session.ID)
				}
				// A subscription checkout carries a plan id; a one-off top-up
				// does not, and setPlan ignores an empty one rather than
				// clearing what somebody is already on.
				setPlan(userID, session.Metadata.PlanID)
				// Kept whether or not this was a subscription: a one-off top-up
				// creates a customer too, and having it means a card saved on
				// one purchase can be managed later without another.
				setCustomer(userID, session.Customer)
			}
		}
	}

	// Handle invoice.payment_succeeded — subscription renewal credits.
	if event.Type == "invoice.payment_succeeded" {
		var invoice struct {
			ID               string `json:"id"`
			SubscriptionData struct {
				Metadata struct {
					UserID  string `json:"user_id"`
					Credits string `json:"credits"`
					PlanID  string `json:"plan_id"`
				} `json:"metadata"`
			} `json:"subscription_details"`
			Subscription string `json:"subscription"`
		}
		if err := json.Unmarshal(event.Data.Object, &invoice); err != nil {
			app.Log("stripe", "invoice parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}

		userID := invoice.SubscriptionData.Metadata.UserID
		var credits int
		fmt.Sscanf(invoice.SubscriptionData.Metadata.Credits, "%d", &credits)

		if userID != "" && credits > 0 {
			// Dedup by invoice ID.
			mutex.Lock()
			if processedSessions[invoice.ID] {
				mutex.Unlock()
				app.Log("stripe", "invoice %s already processed, skipping", invoice.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			processedSessions[invoice.ID] = true
			mutex.Unlock()

			err := AddCredits(userID, credits, quota.OpTopup, map[string]interface{}{
				"source":       "stripe_subscription",
				"invoice_id":   invoice.ID,
				"subscription": invoice.Subscription,
				"plan":         invoice.SubscriptionData.Metadata.PlanID,
			})
			if err != nil {
				app.Log("stripe", "failed to credit subscriber %s: %v", userID, err)
			} else {
				app.Log("stripe", "subscription: credited %d to %s (invoice %s)", credits, userID, invoice.ID)
			}
			// The credits were only ever half of what was paid for. This is the
			// other half: the plan the account is on, which is what decides how
			// many agents it may run and how hard it may write. It rides in the
			// same metadata the credits do and was being read and thrown away.
			setPlan(userID, invoice.SubscriptionData.Metadata.PlanID)
		}
	}

	// Cancelled, or stopped paying.
	//
	// A plan grants standing capacity — agents you keep, a rate you write at —
	// rather than a one-off delivery, so it has to be taken back when the
	// payments stop or it is granted for ever after one month. Nothing here
	// handled this event at all, which was harmless for exactly as long as
	// nothing read Account.Plan.
	//
	// Credits already bought are not clawed back: they were paid for, and a
	// balance is not capacity. What goes is the allowance, back to what an
	// account with no subscription gets — and agents already created stay,
	// because the cap is checked when one is made. Somebody who drops from
	// Premium to nothing keeps their twenty-five and cannot make a
	// twenty-sixth, which is the version of this that does not delete
	// somebody's work over a failed card.
	// A subscription changed — which is how we find out somebody switched plan.
	//
	// Without this, the portal is a way for a customer to silently desync their
	// account from what they are paying: move Pro to Scale and keep five agents
	// while being charged a hundred pounds, or Scale to Pro and keep
	// twenty-five while paying twenty. Neither is a state anybody would report
	// as a bug, which is the worst kind.
	//
	// The price is the truth, not the metadata — see planForPrice. And a plan is
	// only set from a live subscription: this event also fires when a card
	// fails, and an account whose payment is in the past due state has not
	// bought anything new.
	if event.Type == "customer.subscription.updated" {
		var sub struct {
			Status   string `json:"status"`
			Metadata struct {
				UserID string `json:"user_id"`
			} `json:"metadata"`
			Items struct {
				Data []struct {
					Price struct {
						ID         string `json:"id"`
						UnitAmount int    `json:"unit_amount"`
					} `json:"price"`
				} `json:"data"`
			} `json:"items"`
		}
		if err := json.Unmarshal(event.Data.Object, &sub); err != nil {
			app.Log("stripe", "subscription update parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}
		switch sub.Status {
		case "active", "trialing":
			if len(sub.Items.Data) > 0 {
				price := sub.Items.Data[0].Price
				// The Price id first, when the operator has made real ones —
				// it is exact, and it stays right if two plans ever share an
				// amount. The amount is the fallback, and it is all there is
				// for a subscription bought with an inline price.
				id := planForPriceID(price.ID)
				if id == "" {
					id = planForPrice(price.UnitAmount)
				}
				if id != "" {
					setPlan(sub.Metadata.UserID, id)
				} else {
					app.Log("stripe", "%s is on a subscription at %d pence (%s), which is no plan here",
						sub.Metadata.UserID, price.UnitAmount, price.ID)
				}
			}
		case "canceled", "unpaid", "incomplete_expired":
			// Ended for real. A cancellation scheduled for the end of the period
			// is still active until then and is deliberately not caught here —
			// somebody who has paid to the end of the month keeps what they paid
			// for, and the deleted event arrives when it actually ends.
			clearPlan(sub.Metadata.UserID)
		}
	}

	if event.Type == "customer.subscription.deleted" {
		var sub struct {
			ID       string `json:"id"`
			Metadata struct {
				UserID string `json:"user_id"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(event.Data.Object, &sub); err != nil {
			app.Log("stripe", "subscription parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}
		clearPlan(sub.Metadata.UserID)
	}

	w.WriteHeader(http.StatusOK)
}

// verifyStripeSignature verifies the Stripe webhook signature
func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	if sigHeader == "" {
		return false
	}

	// Parse signature header
	var timestamp string
	var signatures []string

	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return false
	}

	// Check timestamp is within tolerance (5 minutes)
	var ts int64
	fmt.Sscanf(timestamp, "%d", &ts)
	now := time.Now().Unix()
	if now-ts > 300 || ts-now > 300 {
		app.Log("stripe", "webhook timestamp out of tolerance: %d vs %d", ts, now)
		return false
	}

	// Verify at least one signature matches
	signedPayload := timestamp + "." + string(payload)
	expectedSig := computeHMAC(signedPayload, secret)

	for _, sig := range signatures {
		if secureCompare(sig, expectedSig) {
			return true
		}
	}

	return false
}

func computeHMAC(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
