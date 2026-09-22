package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"
)

// Plan is the recurring allowance. Operators can override the launch terms;
// changing them only affects new subscriptions, never existing contracts.
type Plan struct {
	Cents   int `json:"cents"`
	Credits int `json:"credits"`
}

func MonthlyPlan() (Plan, bool) {
	priceText, creditText := settings.Get("SUBSCRIPTION_CENTS"), settings.Get("SUBSCRIPTION_CREDITS")
	if priceText == "" && creditText == "" {
		priceText, creditText = "4000", "4000"
	}
	price, e1 := strconv.Atoi(priceText)
	credits, e2 := strconv.Atoi(creditText)
	p := Plan{Cents: price, Credits: credits}
	return p, e1 == nil && e2 == nil && price >= 100 && price <= 100000 && credits > 0 && credits <= 1000000 && TopUpConfigured() && stripeWebhook() != ""
}

// subscriptionMu serialises Stripe reconciliation as well as checkout creation.
// A delayed webhook fetches today's object, never overwrites it with old payload.
var subscriptionMu sync.Mutex
var subscriptions = map[string]subscription{}
var subscriptionsLoadError error
var billingHTTP = &http.Client{Timeout: 20 * time.Second}

const planMarker = "micro_monthly_v1"
const stripeVersion = "2024-06-20"

type subscription struct {
	AccountCreated string `json:"account_created"`
	ID             string `json:"id,omitempty"`
	Customer       string `json:"customer,omitempty"`
	Status         string `json:"status,omitempty"`
	CancelAtEnd    bool   `json:"cancel_at_end,omitempty"`
	PeriodEnd      int64  `json:"period_end,omitempty"`
	Cents          int    `json:"cents,omitempty"`
	Credits        int    `json:"credits,omitempty"`
	PaymentURL     string `json:"payment_url,omitempty"`
	// Persist the exact request before sending. Network failure/restart retries
	// the same Checkout session, including when the response was lost.
	Attempt      string `json:"attempt,omitempty"`
	AttemptAt    int64  `json:"attempt_at,omitempty"`
	CheckoutForm string `json:"checkout_form,omitempty"`
	CheckoutID   string `json:"checkout_id,omitempty"`
	CheckoutURL  string `json:"checkout_url,omitempty"`
}

func init() {
	if err := data.LoadJSON("subscriptions.json", &subscriptions); err != nil && !errors.Is(err, os.ErrNotExist) {
		subscriptionsLoadError = err
	}
	if subscriptions == nil {
		subscriptions = map[string]subscription{}
	}
}

func saveSubscription(id string, s subscription) error {
	old, existed := subscriptions[id]
	if s.ID == "" && s.Attempt == "" && s.AccountCreated == "" {
		delete(subscriptions, id)
	} else {
		subscriptions[id] = s
	}
	if err := data.SaveJSON("subscriptions.json", subscriptions); err != nil {
		if existed {
			subscriptions[id] = old
		} else {
			delete(subscriptions, id)
		}
		return err
	}
	return nil
}

func stripeRequest(ctx context.Context, method, path string, form url.Values, key string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.stripe.com/v1/"+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(stripeSecret(), "")
	req.Header.Set("Stripe-Version", stripeVersion)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := billingHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Stripe returned %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}

type stripeSubscription struct {
	ID            string            `json:"id"`
	Customer      string            `json:"customer"`
	Status        string            `json:"status"`
	CancelAtEnd   bool              `json:"cancel_at_period_end"`
	PeriodEnd     int64             `json:"current_period_end"`
	LatestInvoice string            `json:"latest_invoice"`
	Metadata      map[string]string `json:"metadata"`
}

type stripeInvoice struct {
	ID                  string `json:"id"`
	Subscription        string `json:"subscription"`
	Customer            string `json:"customer"`
	Status              string `json:"status"`
	Currency            string `json:"currency"`
	BillingReason       string `json:"billing_reason"`
	PaymentURL          string `json:"hosted_invoice_url"`
	SubscriptionDetails struct {
		Metadata map[string]string `json:"metadata"`
	} `json:"subscription_details"`
	Lines struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			Type         string `json:"type"`
			Subscription string `json:"subscription"`
			Proration    bool   `json:"proration"`
			Amount       int    `json:"amount"`
			Period       struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"period"`
		} `json:"data"`
	} `json:"lines"`
}

func invoiceGrant(i stripeInvoice) error {
	meta := i.SubscriptionDetails.Metadata
	if meta["plan"] != planMarker {
		return nil
	}
	if i.Status != "paid" {
		return nil
	}
	// Only the original month and full recurring renewals grant usage. A manual
	// invoice or proration is not a second allowance.
	if i.BillingReason != "subscription_create" && i.BillingReason != "subscription_cycle" {
		return nil
	}
	credits, e1 := strconv.Atoi(meta["monthly_credits"])
	price, e2 := strconv.Atoi(meta["monthly_cents"])
	if e1 != nil || e2 != nil || credits <= 0 || price < 100 || i.Currency != "usd" || i.Lines.HasMore || len(i.Lines.Data) != 1 {
		return errors.New("unexpected subscription invoice")
	}
	line := i.Lines.Data[0]
	if line.Type != "subscription" || line.Subscription != i.Subscription || line.Proration || line.Amount != price {
		return errors.New("subscription invoice does not match its plan")
	}
	if !subscriptionOwner(meta) {
		return nil
	}
	return grantMonthly(meta["user_id"], i.Subscription, i.ID, credits, line.Period.Start, line.Period.End)
}

// Reconcile under subscriptionMu. Get the invoice through our pinned API
// version, independent of the webhook endpoint's configured event version.
func reconcileInvoice(ctx context.Context, id string) error {
	var i stripeInvoice
	if err := stripeRequest(ctx, "GET", "invoices/"+url.PathEscape(id), nil, "", &i); err != nil {
		return err
	}
	if i.SubscriptionDetails.Metadata["plan"] != planMarker {
		return nil
	}
	if err := invoiceGrant(i); err != nil {
		return err
	}
	return reconcileSubscription(ctx, i.Subscription, i.SubscriptionDetails.Metadata["user_id"])
}

func reconcileSubscription(ctx context.Context, id, owner string) error {
	if subscriptionsLoadError != nil {
		return errors.New("subscription records are unavailable")
	}
	var remote stripeSubscription
	if err := stripeRequest(ctx, "GET", "subscriptions/"+url.PathEscape(id), nil, "", &remote); err != nil {
		return err
	}
	if remote.Metadata["plan"] != planMarker {
		return nil
	}
	user := remote.Metadata["user_id"]
	if user == "" || (owner != "" && user != owner) {
		return errors.New("subscription belongs to another account")
	}
	if !subscriptionOwner(remote.Metadata) {
		return nil
	}
	s := subscriptions[user]
	if s.Attempt != "" && remote.Metadata["checkout_attempt"] != s.Attempt && s.ID != remote.ID {
		return nil
	}
	// A historic cancelled subscription must not replace the current one.
	if s.ID != "" && s.ID != remote.ID {
		return nil
	}
	cents, e1 := strconv.Atoi(remote.Metadata["monthly_cents"])
	credits, e2 := strconv.Atoi(remote.Metadata["monthly_credits"])
	if e1 != nil || e2 != nil || cents < 100 || credits <= 0 {
		return errors.New("invalid subscription plan")
	}
	s.AccountCreated = remote.Metadata["account_created"]
	s.ID, s.Customer, s.Status = remote.ID, remote.Customer, remote.Status
	s.Cents, s.Credits = cents, credits
	s.CancelAtEnd, s.PeriodEnd = remote.CancelAtEnd, remote.PeriodEnd
	s.PaymentURL = ""
	if remote.LatestInvoice != "" {
		var i stripeInvoice
		if err := stripeRequest(ctx, "GET", "invoices/"+url.PathEscape(remote.LatestInvoice), nil, "", &i); err != nil {
			return err
		}
		if i.Subscription != remote.ID || i.Customer != remote.Customer || i.SubscriptionDetails.Metadata["user_id"] != user {
			return errors.New("invoice ownership mismatch")
		}
		if err := invoiceGrant(i); err != nil {
			return err
		}
		if i.Status == "open" {
			s.PaymentURL = i.PaymentURL
		}
	}
	return saveSubscription(user, s)
}

type subscriptionCheckout struct {
	ID           string            `json:"id"`
	URL          string            `json:"url"`
	Status       string            `json:"status"`
	Mode         string            `json:"mode"`
	Subscription string            `json:"subscription"`
	Metadata     map[string]string `json:"metadata"`
}

func startSubscription(ctx context.Context, acc *auth.Account, origin string) (string, error) {
	subscriptionMu.Lock()
	defer subscriptionMu.Unlock()
	if subscriptionsLoadError != nil {
		return "", errors.New("subscription records are unavailable")
	}
	plan, enabled := MonthlyPlan()
	if !enabled {
		return "", errors.New("subscriptions are not configured")
	}
	if acc.Admin || acc.Agent {
		return "", errors.New("this account has unmetered access")
	}
	if acc.Banned {
		return "", errors.New("this account is unavailable")
	}
	s := subscriptions[acc.ID]
	if s.AccountCreated != strconv.FormatInt(acc.Created.UnixNano(), 10) {
		s = subscription{}
	}
	if s.ID != "" {
		if err := reconcileSubscription(ctx, s.ID, acc.ID); err != nil {
			return "", err
		}
		s = subscriptions[acc.ID]
		if s.Status != "canceled" && s.Status != "incomplete_expired" {
			return "", errors.New("this account already has a subscription")
		}
		// Clear the old subscription only when starting its replacement. Historical
		// invoices still deduplicate in the ledger and retain their original expiry.
		s = subscription{Customer: s.Customer}
	}
	if s.Attempt != "" {
		// After Stripe's idempotency retention, a missing response cannot safely be
		// retried as a new purchase. Reconcile the checkout in Stripe first.
		if s.CheckoutID == "" && time.Now().Unix()-s.AttemptAt >= 23*60*60 {
			return "", errors.New("previous checkout needs reconciliation; contact support")
		}
		if s.CheckoutID != "" {
			var session subscriptionCheckout
			if err := stripeRequest(ctx, "GET", "checkout/sessions/"+url.PathEscape(s.CheckoutID), nil, "", &session); err != nil {
				return "", err
			}
			if session.Metadata["user_id"] != acc.ID {
				return "", errors.New("checkout ownership mismatch")
			}
			if session.Status == "complete" {
				if err := reconcileSubscription(ctx, session.Subscription, acc.ID); err != nil {
					return "", err
				}
				return origin + "/account", nil
			}
			if session.Status == "open" {
				return session.URL, nil
			}
			if session.Status != "expired" {
				return "", errors.New("checkout is still pending")
			}
			s = subscription{Customer: s.Customer}
		}
	}
	if s.Attempt == "" {
		if err := ensureSubscriptionWebhook(ctx, origin); err != nil {
			return "", err
		}
		s.Attempt, s.AttemptAt = uuid.NewString(), time.Now().Unix()
		s.AccountCreated = strconv.FormatInt(acc.Created.UnixNano(), 10)
		form := url.Values{
			"mode": {"subscription"}, "payment_method_types[0]": {"card"},
			"success_url": {origin + "/account/subscription?session_id={CHECKOUT_SESSION_ID}"}, "cancel_url": {origin + "/account"},
			"expires_at":              {strconv.FormatInt(s.AttemptAt+3600, 10)},
			"line_items[0][quantity]": {"1"}, "line_items[0][price_data][currency]": {"usd"},
			"line_items[0][price_data][unit_amount]":               {strconv.Itoa(plan.Cents)},
			"line_items[0][price_data][recurring][interval]":       {"month"},
			"line_items[0][price_data][product_data][name]":        {"Micro Pro"},
			"line_items[0][price_data][product_data][description]": {fmt.Sprintf("%d monthly usage credits. Unused monthly credits expire. Optional prepaid top-ups.", plan.Credits)},
			"metadata[user_id]":                                    {acc.ID}, "metadata[plan]": {planMarker},
			"subscription_data[metadata][account_created]":  {s.AccountCreated},
			"subscription_data[metadata][checkout_attempt]": {s.Attempt},
			"subscription_data[metadata][user_id]":          {acc.ID}, "subscription_data[metadata][plan]": {planMarker},
			"subscription_data[metadata][monthly_credits]": {strconv.Itoa(plan.Credits)},
			"subscription_data[metadata][monthly_cents]":   {strconv.Itoa(plan.Cents)},
		}
		if s.Customer != "" {
			form.Set("customer", s.Customer)
		} else if acc.Customer != "" {
			form.Set("customer", acc.Customer)
		}
		s.CheckoutForm = form.Encode()
		if err := saveSubscription(acc.ID, s); err != nil {
			return "", err
		}
	}
	form, err := url.ParseQuery(s.CheckoutForm)
	if err != nil {
		return "", err
	}
	var session subscriptionCheckout
	if err := stripeRequest(ctx, "POST", "checkout/sessions", form, "subscription-"+s.Attempt, &session); err != nil {
		return "", err
	}
	if session.ID == "" {
		return "", errors.New("checkout is unavailable")
	}
	s.CheckoutID, s.CheckoutURL = session.ID, session.URL
	if err := saveSubscription(acc.ID, s); err != nil {
		return "", err
	}
	if session.URL == "" {
		return "", errors.New("checkout expired; please try again")
	}
	return session.URL, nil
}

func subscriptionEvent(ctx context.Context, kind string, raw json.RawMessage) error {
	if !slices.Contains(subscriptionEvents, kind) {
		return nil
	}
	subscriptionMu.Lock()
	defer subscriptionMu.Unlock()
	var object struct {
		ID           string            `json:"id"`
		Mode         string            `json:"mode"`
		Subscription string            `json:"subscription"`
		Metadata     map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	switch kind {
	case "invoice.paid", "invoice.payment_failed", "invoice.payment_action_required":
		if subscriptionsLoadError != nil {
			return errors.New("subscription records are unavailable")
		}
		return reconcileInvoice(ctx, object.ID)
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		if object.Metadata["plan"] == planMarker {
			return reconcileSubscription(ctx, object.ID, "")
		}
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		if object.Mode == "subscription" && object.Metadata["plan"] == planMarker {
			return reconcileSubscription(ctx, object.Subscription, object.Metadata["user_id"])
		}
	}
	return nil
}

// Account names can be reused after deletion. A historic invoice must never
// grant usage to a new owner of the same name.
func subscriptionOwner(meta map[string]string) bool {
	acc, err := auth.GetAccount(meta["user_id"])
	return err == nil && meta["account_created"] == strconv.FormatInt(acc.Created.UnixNano(), 10)
}

// EndSubscription stops future billing before an administrator deletes an
// account. Failure prevents deletion so the payer retains a way to cancel.
func EndSubscription(ctx context.Context, id string) error {
	subscriptionMu.Lock()
	defer subscriptionMu.Unlock()
	if subscriptionsLoadError != nil {
		return errors.New("subscription records unavailable")
	}
	s := subscriptions[id]
	if s.CheckoutID != "" && s.ID == "" {
		var session subscriptionCheckout
		if err := stripeRequest(ctx, "GET", "checkout/sessions/"+url.PathEscape(s.CheckoutID), nil, "", &session); err != nil {
			return err
		}
		if session.Status == "open" {
			if err := stripeRequest(ctx, "POST", "checkout/sessions/"+url.PathEscape(s.CheckoutID)+"/expire", url.Values{}, "", &session); err != nil {
				return err
			}
		}
		if session.Status == "complete" {
			s.ID = session.Subscription
		}
	}
	if s.ID != "" {
		var remote stripeSubscription
		if err := stripeRequest(ctx, "GET", "subscriptions/"+url.PathEscape(s.ID), nil, "", &remote); err != nil {
			return err
		}
		if remote.Status != "canceled" && remote.Status != "incomplete_expired" {
			if err := stripeRequest(ctx, "DELETE", "subscriptions/"+url.PathEscape(s.ID), nil, "", &remote); err != nil {
				return err
			}
		}
	} else if s.Attempt != "" && s.CheckoutID == "" {
		return errors.New("checkout needs reconciliation before deletion")
	}
	return saveSubscription(id, subscription{})
}
