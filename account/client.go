package account

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/user"
	"mu/service/sms"
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
	state := map[string]any{"id": acc.ID, "name": acc.Name, "email": acc.Email, "email_verified": acc.EmailVerified, "addresses": acc.Addresses, "balance": Balance(acc.ID), "place": acc.Place, "lat": acc.Lat, "lon": acc.Lon, "timezone": acc.Zone, "numbers": sms.Numbers(acc.ID), "pending_number": pending, "phone_enabled": agentNumber() != "", "forwarding": MailForwardingOn(acc.ID), "passkeys": keys, "status": user.Status(acc.ID), "payments": PaymentsEnabled(), "google_enabled": GoogleConfigured(), "google_linked": acc.EmailVerified && acc.Email != ""}
	if r.URL.Path == "/account/billing" || r.URL.Path == "/account/usage" {
		state["admin"] = acc.Admin
		state["daily_credits"] = quota.DailyCredits()
		state["signup_remaining"] = SignupRemaining(acc.ID)
		state["included_today"] = IncludedToday(acc.ID)
		state["monthly"] = Monthly(acc.ID)
		rows := make([]map[string]any, 0)
		for _, tx := range Transactions(acc.ID, 20) {
			rows = append(rows, map[string]any{
				"id": tx.ID, "label": transactionLabel(tx), "amount_label": transactionAmount(tx),
				"balance": tx.Balance, "created_at": tx.CreatedAt,
			})
		}
		state["transactions"] = rows
	}
	app.RespondJSON(w, state)
}
