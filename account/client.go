package account

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/user"
	"mu/service/sms"
	"mu/x402/billing"
	"net/http"
)

// clientAccount exposes only fields used by this account's browser settings.
func clientAccount(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	w.Header().Set("Cache-Control", "private, no-store")
	keys := make([]map[string]any, 0)
	for _, key := range auth.Passkeys(acc.ID) {
		keys = append(keys, map[string]any{"id": key.ID, "name": key.Name, "created": key.Created, "last_used": key.LastUsed})
	}
	pending, _ := sms.Pending(acc.ID)
	state := map[string]any{"id": acc.ID, "name": acc.Name, "email": acc.Email, "email_verified": acc.EmailVerified, "addresses": acc.Addresses, "balance": billing.Balance(acc.ID), "place": acc.Place, "lat": acc.Lat, "lon": acc.Lon, "timezone": acc.Zone, "numbers": sms.Numbers(acc.ID), "pending_number": pending, "phone_enabled": agentNumber() != "", "forwarding": MailForwardingOn(acc.ID), "passkeys": keys, "status": user.Status(acc.ID), "payments": billing.PaymentsEnabled(), "google_enabled": GoogleConfigured(), "google_linked": acc.EmailVerified && acc.Email != ""}
	if r.URL.Path == "/account/billing" || r.URL.Path == "/account/usage" {
		for key, value := range billing.State(acc) {
			state[key] = value
		}
	}
	app.RespondJSON(w, state)
}

// Usage preserves the consumer account representation while billing owns usage rendering.
func Usage(w http.ResponseWriter, r *http.Request) {
	if app.WantsJSON(r) {
		_, acc, err := auth.RequireSession(r)
		if err != nil {
			app.RedirectToLogin(w, r)
			return
		}
		clientAccount(w, r, acc)
		return
	}
	billing.UsageHandler(w, r)
}
