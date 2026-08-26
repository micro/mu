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

	"mu/internal/auth"
	"mu/internal/origin"
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

// Operation types
const (
	OpNewsSearch  = "news_search"
	OpQuranSearch = "quran_search"
	OpVideoSearch = "video_search"
	OpBlogCreate  = "blog_create"
	// OpMailSend is sending a message, wherever it is going.
	//
	// There were two. mail_email was the mailbox answering somebody outside,
	// and mail_send was one account writing to another here, at 0 — on the
	// reasoning that a local message costs this instance nothing to carry.
	// True, and it made spam free: over submission a signed-in account could
	// write to every username on the instance at no cost, no cap, and no
	// filter. Spam is not a fact about which network the recipient is on.
	//
	// So one operation and one price. Writing to yourself, or to your own
	// agent, is not a send and is not charged — see mail.DeliverHere.
	OpMailSend = "mail_send"

	OpSMSSend      = "sms_send"
	OpPlacesSearch = "places_search"
	OpPlacesNearby = "places_nearby"
	// OpRoutesETA keeps the operation id "places_eta" although the service is
	// now routes. The id is what quota.json, every operator's
	// CREDIT_COST_PLACES_ETA and every row of recorded usage are keyed on;
	// renaming it would silently reset the price to the default on upgrade and
	// break the history in two. The tool was renamed, which is visible and has
	// an alias. The price key is not, and does not.
	OpRoutesETA        = "places_eta"
	OpRoutesDirections = "routes_directions"
	OpWeatherForecast  = "weather_forecast"
	OpWeatherPollen    = "weather_pollen"
	OpWebSearch        = "web_search"
	OpWebFetch         = "web_fetch"
	OpBrowserRead      = "browser_read"
	OpBrowserShot      = "browser_shot"
	OpShellRun         = "shell_run"
	OpDBWrite          = "db_write"
	OpImageGenerate    = "image_generate"
	OpTextSummarise    = "text_summarise"
	OpTextExtract      = "text_extract"
	OpTextClassify     = "text_classify"
	OpTextTranslate    = "text_translate"
	OpSocialSearch     = "social_search"
	OpSocialPost       = "social_post"
	OpSocialReply      = "social_reply"
	OpAppCreate        = "app_create"
	OpBlogComment      = "blog_comment"
	OpAppBuild         = "app_build"
	OpAppEdit          = "app_edit"
	OpAppUse           = "app_use"
	OpAppRevenue       = "app_revenue"
	OpTopup            = "topup"
	OpRefund           = "refund"
	OpTransfer         = "transfer"
	OpEscrowHold       = "escrow_hold"
	OpEscrowRelease    = "escrow_release"
	OpEscrowRefund     = "escrow_refund"
)

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
	return OperationCost(operation) > 0
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

	// An agent account is the instance acting for itself, so there is nobody
	// else to bill: the operator is already paying for the instance that runs
	// it. Its calls are recorded like everyone's — the point is that its spend
	// is visible in /usage, not that it is invisible.
	//
	// This used to be got by making the agent an admin, which is exempt for its
	// own reasons. That granted /admin/config, the console and the power to ban, to
	// avoid a balance check. Only an admin can set the flag (see /admin/users),
	// so it is the operator deciding what the instance pays for on its own
	// behalf, not a caller declaring itself free.
	if acc.Agent {
		return true, false, 0, nil
	}

	// If nothing can be charged, nothing is metered (self-hosted instance)
	if !Charging() {
		return true, false, 0, nil
	}

	cost := OperationCost(operation)

	// Check if user has sufficient credits
	balance := BalanceOf(userID)
	if balance >= cost {
		return true, false, cost, nil
	}

	// User needs to top up. The message says what it costs, what they have and
	// where to go: this string is what a person reads when a tool refuses and
	// what an agent reads when it has to explain itself, and "insufficient
	// credits" told neither of them what to do next.
	//
	// Where to go is a whole address when this instance knows its own. The
	// reader here is often a program on another machine, and "/wallet/topup"
	// is only a destination if you already know what it is relative to.
	return false, false, cost, fmt.Errorf(
		"this costs %d credits and your balance is %d — top up at %s", cost, balance, TopupURL())
}

// TopupURL is where to add credits, absolute when this instance knows its own
// address and a path when it does not. See origin.Self.
func TopupURL() string {
	if self := origin.Self(); self != "" {
		return self + "/wallet/topup"
	}
	return "/wallet/topup"
}

// Charge takes payment for an operation that has already happened.
//
// The one way credits move. There were four — ConsumeQuota, ConsumeWith,
// app.Charge and a charge() of service/mail's own — spread over twenty-nine
// call sites, and which one a service used was historical accident. That is how
// the same operation came to be charged twice through one door and not at all
// through another, and it is why nobody could say what anything cost without
// reading five files.
//
// One name. Nothing outside this package may take payment, and
// TestOneWayToCharge holds it.
//
// meta is what the receipt says beyond the operation's name — the query that
// was searched, the number that was texted. Nil where there is nothing to add.
func Charge(userID, operation string, meta map[string]interface{}) error {
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

	// An agent account is the instance acting for itself: recorded, not charged.
	// See CheckQuota for why this is its own rule rather than borrowed from
	// admin.
	if acc.Agent {
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
	cost := OperationCost(operation)
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
		`<a href="/wallet/topup">Top up</a> to continue.</p>` +
		`<p class="text-sm text-muted">1 credit = 1p · <a href="/wallet">Your balance</a></p>` +
		`</div>`
}
