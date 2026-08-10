// Package quota is the price list and the gate in front of it.
//
// A service that calls a provider we pay for has one question — may this caller
// do this, and what does it cost — and it used to have to import the wallet to
// ask. Fifteen of them did, which put a leaf of service/ underneath the core
// that hosts service/, and meant a self-hosted instance had no seam between
// "the tools" and "the money".
//
// So the question lives here and the answer lives in wallet/. This package
// knows what things cost and who is exempt; it does not know what a balance is,
// where credits are kept, or that Stripe and x402 exist. The four things it
// cannot answer are function variables filled in by the server at boot — the
// same way anything else that would be a cycle is wired.
//
// Nothing here imports a service, and no service imports the wallet.
package quota

import (
	"errors"
	"fmt"
	"os"

	"mu/internal/auth"
)

// Wired at boot to the wallet. Unset — a build with no wallet at all — reads as
// an instance that cannot charge, which is the same answer a self-hosted one
// gives, so the gate stays open rather than closed.
var (
	// Enabled reports whether this instance can charge anybody for anything.
	Enabled func() bool

	// Balance is what the caller has to spend.
	Balance func(account string) int

	// Deduct charges the caller for an operation that has already succeeded.
	// The metadata, where there is any, is what the receipt says beyond the
	// operation's name — the query that was searched, and so on.
	Deduct func(account, operation string, amount int, meta map[string]interface{}) error

	// Record notes a call that cost nothing, so free and exempt use still
	// appears in the caller's history.
	Record func(account, operation string)
)

// Charging reports whether this instance can bill anybody for anything. False
// on a self-hosted install with no Stripe and no x402: there is no meter, no
// price and nobody to send it to.
func Charging() bool { return Enabled != nil && Enabled() }

func deduct(account, operation string, amount int, meta map[string]interface{}) error {
	if Deduct == nil {
		return errors.New("nothing on this instance can take payment")
	}
	return Deduct(account, operation, amount, meta)
}

// BalanceOf is what the caller has to spend, or zero on a build with no wallet
// linked in.
func BalanceOf(account string) int {
	if Balance == nil {
		return 0
	}
	return Balance(account)
}

func record(account, operation string) {
	if Record != nil {
		Record(account, operation)
	}
}

// Credit costs per operation (in credits/pennies).
//
// A credit is charged when an operation costs us something to run: a model
// call, or a third-party API we pay for (Atlas Cloud for inference and images,
// Brave for web search, Google for places). Everything else is free.
//
// Actions that only touch this instance's own storage — writing a post, a
// comment, a status update, internal mail between two local users — are 0.
// They have no marginal cost, so charging for them was friction on exactly the
// behaviour the product wants more of.
//
// Abuse control for those does not depend on the charge: auth.CheckPostRate
// runs ahead of the quota check on every write in the central gate, and caps
// new accounts at 10 actions/hour and established ones at 60. That limit was
// always the real defence; the credit was a second, weaker one that also
// taxed ordinary use.
var (
	// Charged — a model call, or a paid third party. The price is the cost
	// with a margin on it, and the margin should be visible in the number
	// rather than a mystery: a model-backed operation costs more than a
	// vendor API call, and a big generation costs more than a small one.

	// Vendor APIs we are billed for per request.
	CostPlacesSearch    = getEnvInt("CREDIT_COST_PLACES_SEARCH", 5) // Google Places text search, ~2.5p
	CostPlacesNearby    = getEnvInt("CREDIT_COST_PLACES_NEARBY", 4) // Google Places nearby, ~2.5p
	CostPlacesETA       = getEnvInt("CREDIT_COST_PLACES_ETA", 3)    // Google Routes, ~0.5p
	CostWeatherForecast = getEnvInt("CREDIT_COST_WEATHER", 1)       // Google Weather
	CostWeatherPollen   = getEnvInt("CREDIT_COST_WEATHER_POLLEN", 1)
	CostWebSearch       = getEnvInt("CREDIT_COST_SEARCH", 2) // Brave, ~0.4p — was 5, a 12x markup

	// Model calls. Ordered by how much model each one actually spends:
	// a chat turn, an agent run that may fan out across tools, and a
	// generation that writes a whole app.
	CostChatQuery  = getEnvInt("CREDIT_COST_CHAT", 5)
	CostAgentQuery = getEnvInt("CREDIT_COST_AGENT", 7)
	// The premium tier routes to Anthropic where the rest use the default
	// provider, which is an order of magnitude more per token. At 9 it was
	// priced 29% above standard for roughly 10-20x the cost — the one place
	// this instance was plausibly underwater.
	CostAgentQueryPremium = getEnvInt("CREDIT_COST_AGENT_PREMIUM", 20)
	CostImageGenerate     = getEnvInt("CREDIT_COST_IMAGE", 15)
	// One generation each. app_build was 100 — a pound, six times the next
	// most expensive thing on the menu, for less model than an agent run
	// costing 7.
	CostAppBuild = getEnvInt("CREDIT_COST_APP_BUILD", 15)
	CostAppEdit  = getEnvInt("CREDIT_COST_APP_EDIT", 8)

	// Sending mail to an external host is the deliberate exception: no
	// invoice arrives for it, because we run the SMTP server. What it spends
	// is the domain's reputation, which is real, not ours to get back, and
	// not something a rate limit prices. mail_send is account-only for the
	// same reason.
	CostExternalEmail = getEnvInt("CREDIT_COST_EMAIL", 4)

	// Free — nothing outside this instance is billed for these. Abuse is a
	// rate limit's job, not a price's. Still overridable by env for operators
	// who want a charge back.
	CostBlogCreate   = getEnvInt("CREDIT_COST_BLOG_CREATE", 0)
	CostBlogComment  = getEnvInt("CREDIT_COST_BLOG_COMMENT", 0)
	CostSocialPost   = getEnvInt("CREDIT_COST_SOCIAL_POST", 0)
	CostSocialReply  = getEnvInt("CREDIT_COST_SOCIAL_REPLY", 0)
	CostAppCreate    = getEnvInt("CREDIT_COST_APP_CREATE", 0)
	CostStreamPost   = getEnvInt("CREDIT_COST_STREAM_POST", 0)
	CostSocialSearch = getEnvInt("CREDIT_COST_SOCIAL", 0)
	CostMailSend     = getEnvInt("CREDIT_COST_MAIL", 0) // local user to local user
	CostDBWrite      = getEnvInt("CREDIT_COST_DB_WRITE", 0)

	// These four were charged for work nothing bills us for.
	//
	// news_search is data.Search against the local index. web_fetch is an
	// http.Get and a readability pass in this process. quran_search calls
	// reminder.dev, which is ours. video_search calls the YouTube Data API,
	// which is free — but quota'd at 10,000 units a day against a search
	// costing 100, so roughly 100 searches a day across every user. That is
	// scarcity, not cost, and rationing it with a price charged the wrong
	// people: see videoSearchLimit.
	CostNewsSearch  = getEnvInt("CREDIT_COST_NEWS", 0)
	CostWebFetch    = getEnvInt("CREDIT_COST_FETCH", 0)
	CostQuranSearch = getEnvInt("CREDIT_COST_QURAN_SEARCH", 0)
	CostVideoSearch = getEnvInt("CREDIT_COST_VIDEO", 0)

	DailyQuota = getEnvInt("DAILY_QUOTA", getEnvInt("FREE_DAILY_QUOTA", 100))
)

// Operation types
const (
	OpNewsSearch        = "news_search"
	OpQuranSearch       = "quran_search"
	OpVideoSearch       = "video_search"
	OpChatQuery         = "chat_query"
	OpBlogCreate        = "blog_create"
	OpMailSend          = "mail_send"
	OpExternalEmail     = "external_email"
	OpPlacesSearch      = "places_search"
	OpPlacesNearby      = "places_nearby"
	OpPlacesETA         = "places_eta"
	OpWeatherForecast   = "weather_forecast"
	OpWeatherPollen     = "weather_pollen"
	OpWebSearch         = "web_search"
	OpWebFetch          = "web_fetch"
	OpDBWrite           = "db_write"
	OpImageGenerate     = "image_generate"
	OpAgentQuery        = "agent_query"
	OpAgentQueryPremium = "agent_query_premium"
	OpSocialSearch      = "social_search"
	OpSocialPost        = "social_post"
	OpSocialReply       = "social_reply"
	OpAppCreate         = "app_create"
	OpStreamPost        = "stream_post"
	OpBlogComment       = "blog_comment"
	OpAppBuild          = "app_build"
	OpAppEdit           = "app_edit"
	OpAppUse            = "app_use"
	OpAppRevenue        = "app_revenue"
	OpTopup             = "topup"
	OpRefund            = "refund"
	OpTransfer          = "transfer"
	OpEscrowHold        = "escrow_hold"
	OpEscrowRelease     = "escrow_release"
	OpEscrowRefund      = "escrow_refund"
)

// getEnvInt gets an environment variable as int with default
func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		fmt.Sscanf(v, "%d", &i)
		if i > 0 {
			return i
		}
	}
	return defaultVal
}

// GetOperationCost returns the credit cost for an operation
func GetOperationCost(operation string) int {
	switch operation {
	case OpNewsSearch:
		return CostNewsSearch
	case OpVideoSearch:
		return CostVideoSearch
	case OpChatQuery:
		return CostChatQuery
	case OpBlogCreate:
		return CostBlogCreate
	case OpMailSend:
		return CostMailSend
	case OpExternalEmail:
		return CostExternalEmail
	case OpPlacesSearch:
		return CostPlacesSearch
	case OpPlacesNearby:
		return CostPlacesNearby
	case OpPlacesETA:
		return CostPlacesETA
	case OpWeatherForecast:
		return CostWeatherForecast
	case OpWeatherPollen:
		return CostWeatherPollen
	case OpQuranSearch:
		return CostQuranSearch
	case OpWebSearch:
		return CostWebSearch
	case OpWebFetch:
		return CostWebFetch
	case OpDBWrite:
		return CostDBWrite
	case OpImageGenerate:
		return CostImageGenerate
	case OpAgentQuery:
		return CostAgentQuery
	case OpAgentQueryPremium:
		return CostAgentQueryPremium
	case OpSocialSearch:
		return CostSocialSearch
	case OpSocialPost:
		return CostSocialPost
	case OpSocialReply:
		return CostSocialReply
	case OpAppCreate:
		return CostAppCreate
	case OpStreamPost:
		return CostStreamPost
	case OpBlogComment:
		return CostBlogComment
	case OpAppBuild:
		return CostAppBuild
	case OpAppEdit:
		return CostAppEdit
	default:
		return 1
	}
}

// Metered reports whether this operation costs the caller anything here.
//
// Two things make it false and they are different: the operation is priced at
// zero, or this instance cannot charge at all. A self-hosted install with no
// Stripe and no x402 has no meter, no price and nobody to bill — CheckQuota has
// always said so, but the gates in front of it did not ask, so a fresh install
// refused anonymous callers with "this call is metered" for weather, which is
// the first thing anybody tries.
func Metered(operation string) bool {
	if !Charging() {
		return false
	}
	return GetOperationCost(operation) > 0
}

// CheckQuota checks if a user can perform an operation
// Returns: canProceed, useQuota (always false now), creditCost, error
func CheckQuota(userID string, operation string) (bool, bool, int, error) {
	// Get account to check admin status
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return false, false, 0, errors.New("account not found")
	}

	// Admins have unlimited access
	if acc.Admin {
		return true, false, 0, nil
	}

	// If nothing can be charged, nothing is metered (self-hosted instance)
	if !Charging() {
		return true, false, 0, nil
	}

	cost := GetOperationCost(operation)

	// Check if user has sufficient credits
	balance := BalanceOf(userID)
	if balance >= cost {
		return true, false, cost, nil
	}

	// User needs to top up. The message says what it costs, what they have and
	// where to go: this string is what a person reads when a tool refuses and
	// what an agent reads when it has to explain itself, and "insufficient
	// credits" told neither of them what to do next.
	return false, false, cost, fmt.Errorf(
		"this costs %d credits and your balance is %d — top up at /wallet", cost, balance)
}

// ConsumeQuota consumes quota for an operation (call after successful operation)
func ConsumeQuota(userID string, operation string) error {
	return ConsumeWith(userID, operation, nil)
}

// ConsumeWith is ConsumeQuota with something to say on the receipt.
func ConsumeWith(userID, operation string, meta map[string]interface{}) error {
	// Get account to check admin status
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return errors.New("account not found")
	}

	// Admins get unlimited access but usage is tracked
	if acc.Admin {
		record(userID, operation)
		return nil
	}

	// A free operation is free: record it and stop. Handing a zero to
	// DeductCredits gets "amount must be positive" back, which the write gate
	// turns into a 402 — so every blog post, comment, reply, status, console
	// note and app, all of which are deliberately priced at zero because they
	// only touch this instance's own storage, was refused for want of credit
	// nobody was being asked for. Admins skip the charge entirely and new
	// accounts were stopped by the post gate first, so the one group who hit it
	// was ordinary established users, on every single write.
	cost := GetOperationCost(operation)
	if cost <= 0 {
		record(userID, operation)
		return nil
	}
	return deduct(userID, operation, cost, meta)
}

// ExceededPage is what a person sees when a priced call is refused for want of
// credit.
//
// It lives here rather than with the wallet because the three services that
// render it — news, search, social — have no other reason to know the wallet
// exists, and importing it for one card is how a service ends up depending on
// money. It says the price and where to go; it reads no balance.
func ExceededPage(cost int) string {
	plural := "s"
	if cost == 1 {
		plural = ""
	}
	return `<div class="card center-card-md">` +
		`<h2>Credits Required</h2>` +
		fmt.Sprintf(`<p>This costs %d credit%s. `, cost, plural) +
		`<a href="/wallet/topup">Add credits</a> to continue.</p>` +
		`<p class="text-sm text-muted">1 credit = 1p · <a href="/wallet">View wallet</a></p>` +
		`</div>`
}
