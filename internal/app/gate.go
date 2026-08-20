package app

// Who has to be signed in, and why.
//
// The rule is one sentence: **an operation that costs this instance money needs
// somebody to bill; one that costs nothing needs nobody.** Everything below is
// that sentence made checkable.
//
// It was already the rule and it was not applied evenly, which is worse than
// having no rule. Weather, news search, web search, web fetch and article
// reading each opened with `auth.RequireSession` and refused a guest outright,
// then asked about credits afterwards. On micro.mu that reads as strictness. On
// a self-hosted instance with no Stripe and no x402 it is nonsense: nothing is
// metered there, nobody can be charged, and the refusal collects an account for
// a call that will never cost anyone anything. Meanwhile /flights, /news and
// /markets answered anybody, so which public page let a guest in came down to
// which handler had been written when.
//
// `quota.Metered` already knew the answer — it is false when the instance
// cannot charge, or the operation is priced at zero. The handlers just were not
// asking it before demanding a session.
//
// The clearest statement of why came from service/places, which reached this
// rule first and then kept it to itself: metering exists because micro.mu is
// run as a product — there is a price list, a balance and a card on file, so a
// charged call needs somebody to charge. None of that is true of an instance
// somebody runs for themselves. A self-hoster who configured their own Google
// key expects to use it; they paid for it. Refusing them until they sign in, on
// their own server, to spend their own quota, is this product's business model
// leaking into somebody else's deployment.
//
// Who may reach an exposed instance is a real question and a different one. It
// is answered by who can sign up and what the instance is open to, not by
// pretending a lookup costs money where nothing is billed.
//
// This is not a relaxation of anything. When an instance can charge and the
// operation costs, the gate is exactly as strict as it was: sign in, then have
// the credits. What changes is that a free call on a free instance stops
// pretending to cost money.

import (
	"net/http"

	"mu/internal/auth"
	"mu/internal/quota"
)

// BillableCaller resolves who to charge for an operation.
//
// It returns the caller's account id and true when the request may proceed. The
// id is empty when nothing is being charged and nobody is signed in, which is a
// perfectly good outcome — a guest asking a free question. Callers must treat an
// empty id as "no one to bill", not as an error.
//
// When it returns false it has already written the response: 401 or a redirect
// to sign in, 402 or the top-up page. The caller returns and writes nothing.
func BillableCaller(w http.ResponseWriter, r *http.Request, op string) (id string, ok bool) {
	if _, acc, err := auth.RequireSession(r); err == nil && acc != nil {
		id = acc.ID
	}

	// Nothing is charged here, so there is nobody who has to be named. Guests
	// on the free path are held by the same per-address rate limit as every
	// other free call — abuse control is auth.CheckPostRate, not the price.
	if !quota.Metered(op) {
		return id, true
	}

	if id == "" {
		if WantsJSON(r) {
			RespondError(w, http.StatusUnauthorized, "sign in — this call is metered on this instance")
		} else {
			RedirectToLogin(w, r)
		}
		return "", false
	}

	canProceed, _, cost, _ := quota.CheckQuota(id, op)
	if !canProceed {
		if WantsJSON(r) {
			RespondError(w, http.StatusPaymentRequired, "Insufficient credits. Top up your wallet to continue.")
		} else {
			Respond(w, r, Response{Title: "Out of credits", HTML: quota.ExceededPage(cost)})
		}
		return "", false
	}
	return id, true
}
