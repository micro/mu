package account

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"mu/internal/auth"
)

var subscriptionEvents = []string{
	"checkout.session.completed", "checkout.session.async_payment_succeeded",
	"invoice.paid", "invoice.payment_failed", "invoice.payment_action_required",
	"customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted",
}

// Existing top-up installations may only subscribe to Checkout events. Add the
// renewal events to this site's enabled endpoint before accepting a subscription.
// Never replace its signing secret or alter another site's endpoint.
var errSubscriptionWebhook = errors.New("subscription webhook setup is unavailable")

func ensureSubscriptionWebhook(ctx context.Context, origin string) error {
	target := strings.TrimRight(origin, "/") + "/stripe/webhook"
	cursor, found := "", false
	for {
		var list struct {
			HasMore bool `json:"has_more"`
			Data    []struct {
				ID     string   `json:"id"`
				URL    string   `json:"url"`
				Status string   `json:"status"`
				Events []string `json:"enabled_events"`
			} `json:"data"`
		}
		path := "webhook_endpoints?limit=100"
		if cursor != "" {
			path += "&starting_after=" + url.QueryEscape(cursor)
		}
		if err := stripeRequest(ctx, "GET", path, nil, "", &list); err != nil {
			return err
		}
		for _, endpoint := range list.Data {
			if endpoint.URL != target || endpoint.Status != "enabled" {
				continue
			}
			found = true
			if slices.Contains(endpoint.Events, "*") {
				continue
			}
			events := slices.Clone(endpoint.Events)
			for _, event := range subscriptionEvents {
				if !slices.Contains(events, event) {
					events = append(events, event)
				}
			}
			if len(events) == len(endpoint.Events) {
				continue
			}
			var updated struct {
				Events []string `json:"enabled_events"`
			}
			if err := stripeRequest(ctx, "POST", "webhook_endpoints/"+url.PathEscape(endpoint.ID), url.Values{"enabled_events[]": events}, "", &updated); err != nil {
				return err
			}
			for _, event := range subscriptionEvents {
				if !slices.Contains(updated.Events, event) && !slices.Contains(updated.Events, "*") {
					return fmt.Errorf("%w: subscription events are unavailable", errSubscriptionWebhook)
				}
			}
		}
		if !list.HasMore {
			break
		}
		if len(list.Data) == 0 || list.Data[len(list.Data)-1].ID == cursor {
			return errors.New("invalid webhook endpoint list")
		}
		cursor = list.Data[len(list.Data)-1].ID
	}
	if !found {
		return fmt.Errorf("%w: enable this site's Stripe webhook endpoint before subscribing", errSubscriptionWebhook)
	}
	return nil
}

// The portal only manages payment details and invoices. Plan changes remain in
// Account, so another Stripe product cannot bypass the allowance contract.
// Protected by subscriptionMu; a restart can recreate it idempotently.
var paymentPortalConfig struct {
	Key [32]byte
	ID  string
}

func subscriptionPortal(ctx context.Context, acc *auth.Account, origin string) (string, error) {
	subscriptionMu.Lock()
	defer subscriptionMu.Unlock()
	s := subscriptions[acc.ID]
	if subscriptionsLoadError != nil || s.ID == "" || s.AccountCreated != strconv.FormatInt(acc.Created.UnixNano(), 10) {
		return "", errors.New("subscription unavailable")
	}
	if err := reconcileSubscription(ctx, s.ID, acc.ID); err != nil {
		return "", err
	}
	s = subscriptions[acc.ID]
	if s.Customer == "" {
		return "", errors.New("payment customer unavailable")
	}
	key := sha256.Sum256([]byte(stripeSecret()))
	if paymentPortalConfig.ID == "" || paymentPortalConfig.Key != key {
		var config struct {
			ID string `json:"id"`
		}
		form := url.Values{
			"features[payment_method_update][enabled]": {"true"},
			"features[invoice_history][enabled]":       {"true"},
			"features[subscription_cancel][enabled]":   {"false"},
			"features[subscription_update][enabled]":   {"false"},
		}
		if err := stripeRequest(ctx, "POST", "billing_portal/configurations", form, "micro-payment-portal-v1", &config); err != nil {
			return "", err
		}
		if config.ID == "" {
			return "", errors.New("payment portal unavailable")
		}
		paymentPortalConfig.Key, paymentPortalConfig.ID = key, config.ID
	}
	var session struct {
		URL string `json:"url"`
	}
	form := url.Values{"customer": {s.Customer}, "configuration": {paymentPortalConfig.ID}, "return_url": {origin + "/account#subscription"}}
	if err := stripeRequest(ctx, "POST", "billing_portal/sessions", form, "", &session); err != nil {
		return "", err
	}
	if !strings.HasPrefix(session.URL, "https://billing.stripe.com/") {
		return "", errors.New("invalid payment portal URL")
	}
	return session.URL, nil
}
