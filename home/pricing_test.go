package home

import (
	"encoding/json"
	"mu/account"
	"mu/internal/quota"
	"net/http/httptest"
	"testing"
)

func takesPayment(t *testing.T) {
	t.Helper()
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_pricing")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_pricing")
}
func pricingData(t *testing.T) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/pricing", nil)
	r.Header.Set("Accept", "application/json")
	PricingHandler(w, r)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestPricingUsesInstanceRates(t *testing.T) {
	takesPayment(t)
	d := pricingData(t)
	if d["payments"] != true || d["topup"] != true || d["question_cost"] != float64(quota.OperationCost(quota.OpAgentRun)) || d["welcome"] != float64(account.WelcomeCredits) || d["daily"] != float64(quota.DailyCredits()) {
		t.Fatal(d)
	}
	if len(d["prices"].([]any)) != len(account.Pricing()) {
		t.Fatal("missing operation prices")
	}
}
func TestPricingOnAnInstanceThatChargesNothing(t *testing.T) {
	if account.PaymentsEnabled() {
		t.Skip("payments configured")
	}
	if pricingData(t)["payments"] != false {
		t.Fatal("unmetered instance claims to charge")
	}
}
