package account

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"mu/internal/data"
)

const txAllowance = "allowance"

// MonthlyAllowance is usage credit, never transferable money. The invoice and
// its service period live in the same authoritative ledger as every debit.
type MonthlyAllowance struct {
	Tier      string    `json:"tier"`
	Credits   int       `json:"credits"`
	Remaining int       `json:"remaining"`
	EndsAt    time.Time `json:"ends_at"`
	Invoice   string    `json:"-"`
}

func metadataInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func Monthly(id string) MonthlyAllowance {
	return withLedger(func(l *ledger) MonthlyAllowance { return monthly(l, id, time.Now().UTC()) })
}

func monthly(_ *ledger, id string, now time.Time) MonthlyAllowance {
	var out MonthlyAllowance
	var start int
	// One paid period at a time. An older webhook cannot replace a newer grant.
	for _, tx := range transactions[id] {
		if tx == nil || tx.Type != txAllowance {
			continue
		}
		from, to := metadataInt(tx.Metadata["period_start"]), metadataInt(tx.Metadata["period_end"])
		if from <= int(now.Unix()) && to > int(now.Unix()) && from >= start {
			invoice, ok := tx.Metadata["invoice"].(string)
			if !ok || invoice == "" {
				continue
			}
			start = from
			tier, _ := tx.Metadata["tier"].(string)
			out = MonthlyAllowance{Tier: normalizedTier(tier), Credits: metadataInt(tx.Metadata["monthly_credits"]), EndsAt: time.Unix(int64(to), 0).UTC(), Invoice: invoice}
		}
	}
	if out.Invoice == "" {
		return out
	}
	used := 0
	for _, tx := range transactions[id] {
		if tx == nil || tx.Metadata["allowance_invoice"] != out.Invoice {
			continue
		}
		switch tx.Type {
		case TxSpend:
			used += metadataInt(tx.Metadata["monthly_credits"])
		case TxRefund:
			used -= metadataInt(tx.Metadata["monthly_credits"])
		}
	}
	out.Remaining = max(0, out.Credits-max(0, used))
	return out
}

func grantMonthly(id, subscription, invoice string, credits int, from, to int64, tiers ...string) error {
	tier := "pro"
	if len(tiers) > 0 {
		tier = normalizedTier(tiers[0])
	}
	if id == "" || subscription == "" || invoice == "" || credits <= 0 || from <= 0 || to <= from {
		return errors.New("invalid monthly allowance")
	}
	return withLedger(func(l *ledger) error {
		for _, list := range transactions {
			for _, tx := range list {
				if tx != nil && tx.Type == txAllowance && tx.Metadata["invoice"] == invoice {
					return nil
				}
			}
		}
		balance := 0
		if w := balances[id]; w != nil {
			balance = w.Balance
		}
		tx := &Transaction{ID: uuid.NewString(), UserID: id, Type: txAllowance, Balance: balance, Operation: "subscription", CreatedAt: time.Now().UTC(), Metadata: map[string]interface{}{
			"tier": tier, "invoice": invoice, "subscription": subscription, "monthly_credits": credits, "period_start": from, "period_end": to,
		}}
		transactions[id] = append(transactions[id], tx)
		if err := data.SaveJSON("transactions.json", transactions); err != nil {
			transactions[id] = transactions[id][:len(transactions[id])-1]
			return err
		}
		return nil
	})
}

func includedAvailable(id string) int {
	return withLedger(func(l *ledger) int {
		now := time.Now().UTC()
		return includedToday(l, id, now) + monthly(l, id, now).Remaining + signupRemaining(l, id)
	})
}

// Consume daily, then monthly, then signup credit, then prepaid. Both web and authenticated APIs
// reach this through quota; x402 settlement and transfers still use money only.
func allowanceDebit(l *ledger, id string, amount int, meta map[string]interface{}) int {
	now := time.Now().UTC()
	daily := min(amount, includedToday(l, id, now))
	plan := monthly(l, id, now)
	monthly := min(amount-daily, plan.Remaining)
	signup := min(amount-daily-monthly, signupRemaining(l, id))
	meta["signup_credits"] = signup
	meta["daily_credits"] = daily
	meta["monthly_credits"] = monthly
	meta["allowance_invoice"] = plan.Invoice
	return amount - daily - monthly - signup
}
