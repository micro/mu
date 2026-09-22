package account

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

func subscriptionSummary(r *http.Request, acc *auth.Account) string {
	subscriptionMu.Lock()
	s := subscriptions[acc.ID]
	subscriptionMu.Unlock()
	if s.AccountCreated != strconv.FormatInt(acc.Created.UnixNano(), 10) {
		s = subscription{}
	}
	if (acc.Admin || acc.Agent) && s.ID == "" {
		return ""
	}
	plan, enabled := MonthlyPlan()
	if !enabled && s.ID == "" {
		return ""
	}
	csrf := app.CSRFField(auth.CSRFToken(r))
	form := func(action, label string) string {
		confirm := ""
		if action == "cancel" {
			confirm = ` data-confirm="Cancel renewal? Your paid allowance remains until the period ends."`
		}
		return `<form method="POST" action="/account/subscription"` + confirm + `>` + csrf + `<input type="hidden" name="action" value="` + action + `"><button class="btn" type="submit">` + label + `</button></form>`
	}
	var body string
	if s.ID != "" {
		allowance := Monthly(acc.ID)
		if allowance.Credits > 0 {
			body = `<p>` + thousands(allowance.Remaining) + ` / ` + thousands(allowance.Credits) + ` monthly credits remaining · expires ` + allowance.EndsAt.Format("2 Jan 2006") + `</p>`
		}
		switch {
		case s.Status == "canceled" || s.Status == "incomplete_expired":
			body += `<p>Subscription ended.</p>`
			if enabled {
				body += `<p>` + money(plan.Cents) + ` / month · ` + thousands(plan.Credits) + ` monthly credits</p>` + form("subscribe", "Subscribe")
			}
		case s.CancelAtEnd:
			body += `<p>Ends ` + time.Unix(s.PeriodEnd, 0).UTC().Format("2 Jan 2006") + `.</p>`
		default:
			body += `<p>` + money(s.Cents) + ` / month · ` + thousands(s.Credits) + ` monthly credits</p>`
			if s.Status == "past_due" || s.Status == "unpaid" || s.Status == "incomplete" {
				body += `<p>Payment required. Monthly credits renew after payment.</p>`
			}
			if strings.HasPrefix(s.PaymentURL, "https://invoice.stripe.com/") {
				body += `<p><a class="btn" href="` + htmlEsc(s.PaymentURL) + `">Pay invoice</a></p>`
			}
			body += form("cancel", "Cancel subscription")
		}
	} else {
		body = `<p>` + money(plan.Cents) + ` / month · ` + thousands(plan.Credits) + ` monthly credits</p>` + form("subscribe", "Subscribe")
	}
	body += `<p class="text-sm text-muted">Daily quota, then monthly credits, then signup credit, then balance. No automatic top-ups.</p>`
	return app.SectionID("subscription", "Subscription", body)
}

// SubscriptionHandler owns checkout return and explicit authenticated billing
// mutations. It adds no account tab or separate billing dashboard.
func SubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodGet {
		id := r.URL.Query().Get("session_id")
		if id == "" {
			http.Redirect(w, r, "/account#subscription", http.StatusSeeOther)
			return
		}
		subscriptionMu.Lock()
		if subscriptions[acc.ID].CheckoutID != id {
			subscriptionMu.Unlock()
			http.Error(w, "Checkout not found", http.StatusNotFound)
			return
		}
		var session subscriptionCheckout
		err = stripeRequest(r.Context(), "GET", "checkout/sessions/"+url.PathEscape(id), nil, "", &session)
		if err == nil && (session.Mode != "subscription" || session.Metadata["plan"] != planMarker || session.Metadata["user_id"] != acc.ID) {
			err = errors.New("checkout belongs to another account")
		}
		if err == nil && session.Status == "complete" {
			err = reconcileSubscription(r.Context(), session.Subscription, acc.ID)
		}
		subscriptionMu.Unlock()
		if err != nil {
			http.Error(w, "Payment could not be confirmed yet. Please try again.", http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/account#subscription", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !auth.StrictCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	switch r.PostForm.Get("action") {
	case "subscribe":
		destination, err := startSubscription(r.Context(), acc, app.BaseURL(r))
		if err != nil {
			app.Log("stripe", "subscription checkout: %v", err)
			http.Error(w, "Unable to start checkout. Please try again or contact support.", http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
	case "cancel":
		subscriptionMu.Lock()
		s := subscriptions[acc.ID]
		if subscriptionsLoadError != nil || s.ID == "" {
			err = errors.New("subscription unavailable")
		} else {
			var remote stripeSubscription
			err = stripeRequest(r.Context(), "POST", "subscriptions/"+url.PathEscape(s.ID), url.Values{"cancel_at_period_end": {"true"}}, "", &remote)
			if err == nil {
				err = reconcileSubscription(r.Context(), s.ID, acc.ID)
			}
		}
		subscriptionMu.Unlock()
		if err != nil {
			http.Error(w, "Cancellation could not be confirmed. Please try again.", http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/account#subscription", http.StatusSeeOther)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

// MonthlyPricingHTML uses the same configuration as Checkout, without claiming
// that an unconfigured plan is available for purchase.
func MonthlyPricingHTML() string {
	p, ok := MonthlyPlan()
	if !ok {
		return ""
	}
	return `<section class="section-stack"><h2>Subscription</h2><p><strong>` + money(p.Cents) + ` / month</strong> · ` + thousands(p.Credits) + ` monthly credits, in addition to the daily quota.</p><p>Shared across the assistant and tools, including API use. Monthly credits reset at renewal. Cancel any time. Top up to continue after your allowance runs out.</p><p><a class="btn" href="/account#subscription">Subscribe</a></p></section>`
}
