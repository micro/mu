package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"mu/internal/pay"
	"mu/internal/quota"
)

// Pay implements guest checkout without reusing or updating the credited
// account's Stripe customer. Money funds the existing account credit ledger.
type Pay struct{}

func (Pay) Checkout(ctx context.Context, r pay.Record) (string, string, error) {
	if !StripeEnabled() || stripeWebhook() == "" {
		return "", "", errors.New("Mu Pay requires Stripe and a configured webhook")
	}
	s := r.Session
	form := url.Values{
		"mode":                                   {"payment"},
		"payment_method_types[0]":                {"card"},
		"success_url":                            {s.CheckoutUrl + "?return=1"},
		"cancel_url":                             {r.ReturnURL},
		"expires_at":                             {fmt.Sprint(r.Expires)},
		"line_items[0][quantity]":                {fmt.Sprint(s.Participants)},
		"line_items[0][price_data][currency]":    {s.Currency},
		"line_items[0][price_data][unit_amount]": {fmt.Sprint(s.Amount / s.Participants)},
		"line_items[0][price_data][product_data][name]":        {"Private agent session"},
		"line_items[0][price_data][product_data][description]": {fmt.Sprintf("%d minutes per session; starts when the host presses Start", s.Minutes)},
		"metadata[mu_pay]": {s.Id}, "metadata[owner]": {r.Owner}, "metadata[agent]": {s.Agent},
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := stripeRequest(ctx, "POST", "checkout/sessions", form, "mu-pay-"+s.Id, &out); err != nil {
		return "", "", err
	}
	if out.ID == "" || out.URL == "" {
		return "", "", errors.New("Stripe returned no checkout")
	}
	return out.ID, out.URL, nil
}
func (Pay) Credit(ctx context.Context, r pay.Record) error {
	_, err := CreditOnce(r.Owner, int(r.Session.Credits), quota.OpTopup, "mu-pay:"+r.StripeID, map[string]any{"source": "mu_pay", "session": r.Session.Id, "amount": r.Session.Amount, "currency": r.Session.Currency})
	return err
}
func payEvent(ctx context.Context, kind, account string, raw json.RawMessage) (bool, error) {
	var event struct {
		ID            string            `json:"id"`
		Mode          string            `json:"mode"`
		PaymentStatus string            `json:"payment_status"`
		AmountTotal   int32             `json:"amount_total"`
		Currency      string            `json:"currency"`
		Metadata      map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return false, err
	}
	id := event.Metadata["mu_pay"]
	if id == "" {
		return false, nil
	}
	if account != "" {
		return true, errors.New("Mu Pay does not accept connected-account events")
	}
	if kind != "checkout.session.completed" && kind != "checkout.session.async_payment_succeeded" {
		return true, nil
	}
	if event.Mode != "payment" {
		return true, errors.New("unexpected Mu Pay checkout mode")
	}
	return true, pay.Settle(ctx, pay.Settlement{ID: id, StripeID: event.ID, Owner: event.Metadata["owner"], Agent: event.Metadata["agent"], Amount: event.AmountTotal, Currency: event.Currency, Status: event.PaymentStatus})
}
