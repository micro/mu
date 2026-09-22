package account

import (
	"github.com/google/uuid"
	"mu/internal/auth"
	"mu/internal/data"
	"time"
)

// SignupCredits is the one-time usage allowance for a new personal account.
// It is not money: it cannot be transferred or establish paid-account trust.
const SignupCredits = 100
const txSignup = "signup_allowance"

func grantSignup(id string) error {
	acc, err := auth.GetAccount(id)
	if err != nil {
		return err
	}
	if !PaymentsEnabled() || acc.Admin || acc.Agent || acc.Unclaimed {
		return nil
	}
	return withLedger(func(l *ledger) error {
		for _, tx := range transactions[id] {
			if tx != nil && (tx.Type == txSignup || isWelcome(tx)) {
				return nil
			}
		}
		balance := 0
		if w := balances[id]; w != nil {
			balance = w.Balance
		}
		transactions[id] = append(transactions[id], &Transaction{
			ID: uuid.NewString(), UserID: id, Type: txSignup, Operation: OpWelcome,
			Balance: balance, CreatedAt: time.Now().UTC(),
			Metadata: map[string]interface{}{"signup_credits": SignupCredits},
		})
		if err := data.SaveJSON("transactions.json", transactions); err != nil {
			transactions[id] = transactions[id][:len(transactions[id])-1]
			return err
		}
		return nil
	})
}

func signupRemaining(_ *ledger, id string) int {
	granted, used := 0, 0
	for _, tx := range transactions[id] {
		if tx == nil {
			continue
		}
		amount := metadataInt(tx.Metadata["signup_credits"])
		switch tx.Type {
		case txSignup:
			granted += amount
		case TxSpend:
			used += amount
		case TxRefund:
			used -= amount
		}
	}
	return max(0, granted-max(0, used))
}

// SignupRemaining reports unused signup credit without creating a grant.
func SignupRemaining(id string) int {
	return withLedger(func(l *ledger) int { return signupRemaining(l, id) })
}
