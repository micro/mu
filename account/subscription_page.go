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
	plan, enabled := MonthlyPlan()
	if !enabled && s.ID == "" {
		return ""
	}
	if (acc.Admin || acc.Agent) && s.ID == "" {
		return app.SectionID("subscription", "Plan", `<p>Included · no usage charges for this account.</p>`)
	}
	csrf := app.CSRFField(auth.CSRFToken(r))
	form := func(action, label string) string {
		confirm := ""
		if action == "cancel" {
			confirm = ` data-confirm="Cancel Pro renewal? Your paid allowance remains until the period ends."`
		}
		if action == "resume" {
			confirm = ` data-confirm="Resume Pro at ` + money(s.Cents) + ` per month?"`
		}
		return `<form method="POST" action="/account/subscription"` + confirm + `>` + csrf + `<input type="hidden" name="action" value="` + action + `"><button class="btn" type="submit">` + label + `</button></form>`
	}
	allowance := Monthly(acc.ID)
	body := `<p><strong>Free</strong></p>`
	if allowance.Credits > 0 {
		body = `<p><strong>Pro</strong> · ` + money(s.Cents) + `/month</p><p>` + thousands(allowance.Remaining) + ` / ` + thousands(allowance.Credits) + ` credits remaining</p>`
	}
	ongoing := s.ID != "" && s.Status != "canceled" && s.Status != "incomplete_expired"
	if ongoing {
		switch {
		case s.CancelAtEnd:
			body += `<p>Ends ` + time.Unix(s.PeriodEnd, 0).UTC().Format("2 Jan 2006") + `.</p>`
		case s.Status == "past_due" || s.Status == "unpaid" || s.Status == "incomplete":
			body += `<p>Payment required to renew Pro.</p>`
		case allowance.Credits > 0:
			body += `<p>Renews ` + allowance.EndsAt.Format("2 Jan 2006") + `.</p>`
		default:
			body += `<p>Pro payment is being confirmed.</p>`
		}
		body += `<div class="form-actions">`
		if strings.HasPrefix(s.PaymentURL, "https://invoice.stripe.com/") {
			body += `<a class="btn" href="` + htmlEsc(s.PaymentURL) + `">Pay invoice</a>`
		}
		if s.Customer != "" {
			body += form("manage", "Payment details")
		}
		if s.CancelAtEnd {
			body += form("resume", "Resume Pro")
		} else {
			body += form("cancel", "Cancel renewal")
		}
		body += `</div>`
	} else if enabled {
		if allowance.Credits > 0 {
			body += `<p>Ends ` + allowance.EndsAt.Format("2 Jan 2006") + `.</p>`
		}
		body += `<p>Pro · ` + money(plan.Cents) + `/month · ` + thousands(plan.Credits) + ` monthly credits</p>`
		label := "Upgrade to Pro"
		if r.URL.Query().Get("plan") == "pro" {
			label = "Continue to payment"
		}
		body += `<div class="form-actions">` + form("subscribe", label)
		if s.ID != "" && s.Customer != "" {
			body += form("manage", "Payment details")
		}
		body += `</div>`
	}
	if allowance.Credits > 0 {
		body += `<p><a href="/events?view=brief">Daily briefs</a></p>`
	} else if enabled {
		body += `<p class="text-sm text-muted">For your daily brief and assistant. Renews monthly. Cancel any time.</p>`
	}
	return app.SectionID("subscription", "Plan", body)
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
			message := "Checkout is temporarily unavailable. Please try again."
			if errors.Is(err, errSubscriptionWebhook) {
				message = "Pro payment setup needs attention. Please contact support."
			}
			var failure *stripeAPIError
			if errors.As(err, &failure) && failure.Status >= 400 && failure.Status < 500 && failure.Status != 429 {
				message = "Pro payment setup needs attention. Please contact support."
			}
			body := app.Problem(message)
			if failure != nil && failure.RequestID != "" {
				body += `<p>Payment reference: <code>` + htmlEsc(failure.RequestID) + `</code></p>`
			}
			body += `<div class="form-actions"><a href="/account#subscription">Back to Account</a><a href="/contact">Contact support</a></div>`
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			app.Respond(w, r, app.Response{Title: "Checkout unavailable", HTML: body})
			return
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
	case "manage":
		destination, err := subscriptionPortal(r.Context(), acc, app.BaseURL(r))
		if err != nil {
			app.Log("stripe", "payment details: %v", err)
			http.Error(w, "Payment details are unavailable. Please try again.", http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
	case "cancel", "resume":
		subscriptionMu.Lock()
		s := subscriptions[acc.ID]
		if subscriptionsLoadError != nil || s.ID == "" || s.AccountCreated != strconv.FormatInt(acc.Created.UnixNano(), 10) {
			err = errors.New("subscription unavailable")
		} else {
			var remote stripeSubscription
			err = stripeRequest(r.Context(), "POST", "subscriptions/"+url.PathEscape(s.ID), url.Values{"cancel_at_period_end": {strconv.FormatBool(r.PostForm.Get("action") == "cancel")}}, "", &remote)
			if err == nil {
				err = reconcileSubscription(r.Context(), s.ID, acc.ID)
			}
		}
		subscriptionMu.Unlock()
		if err != nil {
			http.Error(w, "Plan change could not be confirmed. Please try again.", http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/account#subscription", http.StatusSeeOther)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

// MonthlyPricingHTML uses the same configuration as Checkout, without claiming
// that an unconfigured plan is available for purchase.
func MonthlyPricingHTML(r *http.Request) string {
	p, ok := MonthlyPlan()
	if !ok {
		return ""
	}
	destination, label := "/account?plan=pro#subscription", "Get Pro"
	if _, acc, err := auth.RequireSession(r); err != nil {
		destination = "/signup?redirect=" + url.QueryEscape(destination)
	} else {
		if Monthly(acc.ID).Credits > 0 {
			destination, label = "/account#subscription", "Your plan"
		}
	}

	return `<section class="plan-section section-stack"><h2>Pro</h2><p><strong>` + money(p.Cents) + `/month</strong></p><p>A recurring credit allowance for regular use of Micro and its services.</p><p>` + thousands(p.Credits) + ` monthly credits for assistant replies and paid service operations, in addition to any free daily credits.</p><p>The same services as Free and PAYG, with credits added each billing month. Unused monthly credits expire. Renews monthly; cancel any time.</p><p><a class="btn" href="` + htmlEsc(destination) + `">` + label + `</a></p></section>`
}
