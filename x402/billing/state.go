package billing

import (
	"mu/internal/auth"
	"mu/internal/quota"
)

func State(acc *auth.Account) map[string]any {
	state := map[string]any{"id": acc.ID, "balance": Balance(acc.ID), "payments": PaymentsEnabled()}
	state["admin"] = acc.Admin
	state["daily_credits"] = quota.DailyCredits()
	state["signup_remaining"] = SignupRemaining(acc.ID)
	state["included_today"] = IncludedToday(acc.ID)
	state["monthly"] = Monthly(acc.ID)
	rows := make([]map[string]any, 0)
	for _, tx := range Transactions(acc.ID, 20) {
		rows = append(rows, map[string]any{
			"id": tx.ID, "label": TransactionLabel(tx), "amount_label": TransactionAmount(tx),
			"balance": tx.Balance, "created_at": tx.CreatedAt,
		})
	}
	state["transactions"] = rows
	return state
}
