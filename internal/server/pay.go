package server

// Who is being charged for a tool call, and whether they can afford it.
//
// The payment gate used to ask one question — is there a valid token — and
// treat "yes" as the end of it. So the x402 challenge, which carries the price,
// the asset, the chain and the address to pay, only ever reached a caller with
// no account at all. Signing in put you in the credits lane permanently: an
// agent that ran out was refused with a sentence about a web page, over a
// protocol it cannot browse, having never been told that paying per call was
// something this server does.
//
// The perverse result was that signing up made an agent's position strictly
// worse. Anonymous, it got ten free trial calls and then a machine-readable
// price. Signed in, it got a dead end. This asks the question that was missing:
// not "is there somebody" but "is there somebody who can pay for this".

import (
	"fmt"
	"net/http"

	"mu/internal/auth"
	"mu/internal/quota"
)

// payer reports who is calling, whether they must be shown a payment challenge,
// and what to tell them.
//
// An empty reason means the challenge's own wording, which is right for a
// caller with no account: it already names both ways forward. A signed-in
// caller needs different words, because "sign in" is not advice they can act
// on.
//
// The name is for the usage record. Every refusal used to be filed under
// "guest" whoever it was, so the one figure that would show signed-in callers
// hitting a wall looked like anonymous traffic.
func payer(r *http.Request, token, op string) (who string, blocked bool, reason string) {
	if err := auth.ValidateToken(token); err != nil {
		return "guest", true, ""
	}

	sess, err := auth.GetSession(r)
	if err != nil {
		return "guest", true, ""
	}

	canProceed, _, cost, _ := quota.CheckQuota(sess.Account, op)
	if canProceed {
		return sess.Account, false, ""
	}

	// Signed in and out of credits. Both ways on are named, and the address to
	// top up at is a whole one — the reader is usually not in a browser and may
	// not be on this machine.
	return sess.Account, true, fmt.Sprintf(
		"This costs %d credits and your balance is %d. Top up at %s, or pay for this "+
			"call with an X-PAYMENT header — see accepts.",
		cost, quota.BalanceOf(sess.Account), quota.TopupURL())
}
