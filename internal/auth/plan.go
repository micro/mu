package auth

// SubscriptionTier is supplied by Account from the paid-period ledger.
// Services consume entitlements without importing billing or agents.
var SubscriptionTier func(string) string

func Plan(owner string) string {
	if SubscriptionTier != nil {
		return SubscriptionTier(owner)
	}
	return "free"
}
