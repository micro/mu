package account

import (
	"encoding/json"
	"errors"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/quota"

	"github.com/google/uuid"
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
		if tx == nil || (tx.Type != TxSpend && tx.Type != TxRefund) {
			continue
		}
		txDay := tx.CreatedAt.UTC().Format("2006-01-02")
		if original, ok := tx.Metadata["allowance_day"].(string); ok {
			txDay = original
		}
		if txDay != day {
			continue
		}
		sign := 1
		if tx.Type == TxRefund {
			sign = -1
		}
		switch n := tx.Metadata["daily_credits"].(type) {
		case int:
			used += sign * max(0, n)
		case float64:
			used += sign * max(0, int(n))
		case json.Number:
			v, _ := n.Int64()
			used += sign * max(0, int(v))
		}
	}
	return max(0, quota.DailyCredits()-used)
}

// reserveIncluded writes the debit before returning permission to call a
// provider. Concurrent calls cannot spend the same included or funded credit.
func reserveIncluded(id, operation string, amount int) (func(bool) error, error) {
	var receipt string
	err := withLedger(func(l *ledger) error {
		included := min(amount, includedToday(l, id, time.Now().UTC()))
		if balances[id] == nil {
			balances[id] = &Credits{UserID: id, Currency: "USD"}
		}
		if balances[id].Balance < amount-included {
			return errors.New(quota.Shortfall(amount, balances[id].Balance+included))
		}
		if err := deductCredits(l, id, amount-included, operation, map[string]interface{}{
			"daily_credits": included, "pending": true,
		}); err != nil {
			return err
		}
		receipt = transactions[id][len(transactions[id])-1].ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return func(success bool) error {
		return withLedger(func(l *ledger) error { return settleIncluded(l, id, receipt, success) })
	}, nil
}

// Settlement and any refund share one authoritative ledger write. Repeating
// settlement is harmless. Allowance refunds belong to the original UTC day.
func settleIncluded(_ *ledger, id, receipt string, success bool) error {
	for _, tx := range transactions[id] {
		if tx == nil || tx.ID != receipt || tx.Metadata["pending"] != true {
			continue
		}
		w := balances[id]
		if w == nil {
			return errors.New("reservation balance is unavailable")
		}
		previous := *w
		length := len(transactions[id])
		tx.Metadata["pending"] = false
		if !success {
			w.Balance -= tx.Amount
			w.UpdatedAt = time.Now().UTC()
			transactions[id] = append(transactions[id], &Transaction{
				ID: uuid.New().String(), UserID: id, Type: TxRefund,
				Amount: -tx.Amount, Balance: w.Balance, Operation: tx.Operation,
				CreatedAt: w.UpdatedAt, Metadata: map[string]interface{}{
					"reservation": receipt, "daily_credits": tx.Metadata["daily_credits"],
					"allowance_day": tx.CreatedAt.UTC().Format("2006-01-02"),
				},
			})
		}
		if err := data.SaveJSON("transactions.json", transactions); err != nil {
			tx.Metadata["pending"] = true
			*w = previous
			transactions[id] = transactions[id][:length]
			return err
		}
		if !success {
			if err := data.SaveJSON("wallets.json", balances); err != nil {
				app.Log("account", "refund balance cache: %v", err)
			}
		}
		return nil
	}
	return nil
}

// Startup has no running calls. Refund reservations left by an interrupted run
// rather than charging a person for an answer the server never completed.
func recoverIncluded() {
	withLedger(func(l *ledger) bool {
		for id, list := range transactions {
			for _, tx := range list {
				if tx != nil && tx.Metadata["pending"] == true {
					if err := settleIncluded(l, id, tx.ID, false); err != nil {
						app.Log("account", "recover reservation %s: %v", tx.ID, err)
					}
				}
			}
		}
		return true
	})
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
		if balances[id].Balance < amount-included {
			return errors.New(quota.Shortfall(amount, balances[id].Balance+included))
		}
		return deductCredits(l, id, amount-included, operation, meta)
	})
}
