package gateway

import (
	"fmt"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	x402 "mu/x402/payment"
	"net/http"
)

func Check(r *http.Request, op string) (bool, int, error) {
	// Nothing to charge, nobody to charge it to. A free tool has no
	// business asking who is calling: news, web fetch, quran and video
	// search are priced at zero because nothing bills us for them, and an
	// anonymous caller was being turned away from all four with "this call
	// is metered". It is not.
	//
	// This is the same mistake as the x402 gate in the HTTP layer, made
	// independently here, which is why fixing that one alone left free
	// tools unreachable. Both now ask what it costs before asking who you
	// are.
	if !quota.Metered(op) {
		// Open, not unguarded. Credits price what a call costs us and rate
		// limits stop bots — see the cost block in internal/quota. A free call
		// is charged nothing, so the limit is the only one of the two
		// doing any work here, and it applies to guests because a
		// signed-in caller is already accountable.
		if _, err := auth.GetSession(r); err != nil && !app.GuestAllowed(r) {
			return false, 0, fmt.Errorf("too many free calls from this address — " +
				"sign in at /account/tokens to keep going, or wait a few minutes")
		}
		return true, 0, nil
	}
	// Check for x402 payment (bypasses auth + credits).
	// A wallet hint header is not proof of identity or payment.
	// Every priced x402 request must pass verification before execution.
	if r.Context().Value(x402.X402ContextKey) != nil {
		// Verify, do not settle. The money moves once there is an answer
		// to hand back — see x402.Finish. Everything that can refuse a
		// caller happens here, so a verified payment is a promise that
		// settling will work rather than a charge already taken.
		if _, err := x402.Verify(r, op, r.URL.Path); err != nil {
			return false, 0, fmt.Errorf("x402 payment failed: %w", err)
		}
		return true, 0, nil
	}
	sess, err := auth.GetSession(r)
	if err != nil {
		// Not "authentication required", which is what an account-scoped
		// tool answers with a 401 and a WWW-Authenticate header telling a
		// client where to sign in. This is a different condition wearing
		// the same words: the call is metered, and there is nobody to
		// charge. Signing in is one answer; paying is the other, and a
		// client told to authenticate would never find the second.
		return false, 0, fmt.Errorf("this call is metered: sign in so it can be charged to your credits, or send an x402 payment")
	}
	canProceed, _, cost, err := quota.CheckQuota(sess.Account, op)
	return canProceed, cost, err
}

func CheckAgent(r *http.Request, op string) (bool, int, error) {
	// Free is free here too — the third copy of this decision. See the
	// note on api.QuotaCheck above.
	if !quota.Metered(op) {
		return true, 0, nil
	}
	// Check for x402 payment (bypasses auth + credits)
	if r.Context().Value(x402.X402ContextKey) != nil {
		// Verify, do not settle. The money moves once there is an answer
		// to hand back — see x402.Finish. Everything that can refuse a
		// caller happens here, so a verified payment is a promise that
		// settling will work rather than a charge already taken.
		if _, err := x402.Verify(r, op, r.URL.Path); err != nil {
			return false, 0, fmt.Errorf("x402 payment failed: %w", err)
		}
		return true, 0, nil
	}
	sess, err := auth.GetSession(r)
	if err != nil {
		// Not "authentication required", which is what an account-scoped
		// tool answers with a 401 and a WWW-Authenticate header telling a
		// client where to sign in. This is a different condition wearing
		// the same words: the call is metered, and there is nobody to
		// charge. Signing in is one answer; paying is the other, and a
		// client told to authenticate would never find the second.
		return false, 0, fmt.Errorf("this call is metered: sign in so it can be charged to your credits, or send an x402 payment")
	}
	canProceed, _, cost, err := quota.CheckQuota(sess.Account, op)
	return canProceed, cost, err
}
