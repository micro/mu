package account

import (
	"context"
	"mu/internal/auth"
	"net/http"
	"testing"
	"time"
)

func TestTierFollowsPaidPeriodNotRemainingBalance(t *testing.T) {
	acc := subscriptionFixture(t)
	if Tier(acc.ID) != "free" {
		t.Fatal("unpaid account has benefits")
	}
	now := time.Now()
	if err := grantMonthly(acc.ID, "sub_starter", "in_starter", 1000, now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix(), "starter"); err != nil {
		t.Fatal(err)
	}
	if auth.Plan(acc.ID) != "starter" {
		t.Fatal("starter entitlement missing")
	}
	settle, err := reserveIncluded(acc.ID, "test", IncludedToday(acc.ID)+1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(true); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 0 || Tier(acc.ID) != "starter" {
		t.Fatal("credit exhaustion removed benefits")
	}
	if err := grantMonthly(acc.ID, "sub_old", "in_expired", 4000, now.Add(-48*time.Hour).Unix(), now.Add(-24*time.Hour).Unix(), "pro"); err != nil {
		t.Fatal(err)
	}
	if Tier(acc.ID) != "starter" {
		t.Fatal("expired pro overrode starter")
	}
}
func TestStarterAndProDefaults(t *testing.T) {
	subscriptionFixture(t)
	for _, key := range []string{"STARTER_CENTS", "STARTER_CREDITS", "SUBSCRIPTION_CENTS", "SUBSCRIPTION_CREDITS"} {
		t.Setenv(key, "")
	}
	for _, want := range []struct {
		tier           string
		cents, credits int
	}{{"starter", 1200, 1000}, {"pro", 4500, 4000}} {
		p, ok := SubscriptionPlan(want.tier)
		if !ok || p.Cents != want.cents || p.Credits != want.credits {
			t.Fatalf("%s: %+v %v", want.tier, p, ok)
		}
	}
	if _, ok := SubscriptionPlan("unknown"); ok {
		t.Fatal("unknown plan accepted")
	}
}

func TestStarterCheckoutAndInvoiceKeepTheirTier(t *testing.T) {
	acc := subscriptionFixture(t)
	t.Setenv("STARTER_CENTS", "")
	t.Setenv("STARTER_CREDITS", "")
	checkout := subscriptionCheckout{ID: "cs_starter", URL: "https://checkout.stripe.com/c/starter", Status: "open", Mode: "subscription", Metadata: map[string]string{"user_id": acc.ID, "plan": planMarker, "tier": "starter"}}
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/webhook_endpoints" {
			return stripeResponse(map[string]any{"data": []any{map[string]any{"id": "we_test", "url": "https://micro.test/stripe/webhook", "status": "enabled", "enabled_events": []string{"*"}}}}), nil
		}
		if r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			for k, want := range map[string]string{"metadata[tier]": "starter", "subscription_data[metadata][tier]": "starter", "line_items[0][price_data][unit_amount]": "1200", "subscription_data[metadata][monthly_credits]": "1000"} {
				if r.PostForm.Get(k) != want {
					t.Fatalf("%s: got %s", k, r.PostForm.Get(k))
				}
			}
		}
		return stripeResponse(checkout), nil
	})}
	if _, err := startSubscription(context.Background(), acc, "https://micro.test", "starter"); err != nil {
		t.Fatal(err)
	}
	if _, err := startSubscription(context.Background(), acc, "https://micro.test", "pro"); err == nil {
		t.Fatal("reused Starter checkout for Pro")
	}
	invoice := invoiceFixture(acc, "in_starter_paid", time.Now().Add(-time.Hour))
	invoice.SubscriptionDetails.Metadata["tier"] = "starter"
	invoice.SubscriptionDetails.Metadata["monthly_cents"] = "1200"
	invoice.SubscriptionDetails.Metadata["monthly_credits"] = "1000"
	invoice.Lines.Data[0].Amount = 1200
	if err := invoiceGrant(invoice); err != nil {
		t.Fatal(err)
	}
	if Tier(acc.ID) != "starter" || Monthly(acc.ID).Credits != 1000 {
		t.Fatal("Starter payment did not grant Starter terms")
	}
}
