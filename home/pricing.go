package home

import (
	"encoding/json"
	"mu/account"
	"mu/internal/quota"
	"mu/web"
	"net/http"
)

func PricingHandler(w http.ResponseWriter, r *http.Request) {
	if web.Page(w, r, "Pricing") {
		return
	}
	type limit struct {
		Label string `json:"label"`
		Limit int    `json:"limit"`
	}
	limits := []limit{}
	for _, p := range quota.Prices() {
		if n := quota.DailyLimit(p.Op); n != quota.NoLimit {
			limits = append(limits, limit{p.Label, n})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	json.NewEncoder(w).Encode(map[string]any{
		"payments": account.PaymentsEnabled(), "topup": account.TopUpConfigured(),
		"question_cost": quota.OperationCost(quota.OpAgentRun), "welcome": account.WelcomeCredits,
		"daily": quota.DailyCredits(), "prices": account.Pricing(), "limits": limits,
	})
}
