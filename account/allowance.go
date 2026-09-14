package account

import (
	"encoding/json"
	"time"

	"mu/internal/quota"
)

// IncludedToday is an account's remaining daily budget. Usage lives in the
// transaction ledger, so a restart cannot grant a second day's allowance.
func IncludedToday(id string) int {
	return withLedger(func(l *ledger) int { return includedToday(l, id, time.Now().UTC()) })
}

func includedToday(_ *ledger, id string, now time.Time) int {
	used := 0
	day := now.Format("2006-01-02")
	for _, tx := range transactions[id] {
		if tx == nil || tx.Type != TxSpend || tx.CreatedAt.UTC().Format("2006-01-02") != day {
			continue
		}
		switch n := tx.Metadata["daily_credits"].(type) {
		case int:
			used += max(0, n)
		case float64:
			used += max(0, int(n))
		case json.Number:
			v, _ := n.Int64()
			used += max(0, int(v))
		}
	}
	return max(0, quota.DailyCredits()-used)
}

// Only ordinary metered operations use the allowance. Direct ledger transfers,
// app-author payments and escrow continue to require a funded balance.
func chargeIncluded(id string, amount int, operation string, metadata map[string]interface{}) error {
	return withLedger(func(l *ledger) error {
		included := min(amount, includedToday(l, id, time.Now().UTC()))
		meta := make(map[string]interface{}, len(metadata)+1)
		for k, v := range metadata {
			meta[k] = v
		}
		meta["daily_credits"] = included
		if balances[id] == nil {
			balances[id] = &Credits{UserID: id, Currency: "USD"}
		}
		return deductCredits(l, id, amount-included, operation, meta)
	})
}
