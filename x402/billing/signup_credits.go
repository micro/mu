package billing

// SignupCredits is retained for API compatibility. New accounts receive no grant.
const SignupCredits = 0
const txSignup = "signup_allowance"

// GrantSignup no longer issues credit, including retries for older accounts.
func GrantSignup(id string) error { return nil }

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

func RetrySignup(id string) {}
