package account

import "mu/internal/auth"

// Missing tier metadata belongs to the original Pro subscription.
func normalizedTier(tier string) string {
	if tier == "starter" {
		return tier
	}
	return "pro"
}

// Tier follows a paid service period, even after cancellation or credit exhaustion.
func Tier(owner string) string {
	acc, err := auth.GetAccount(owner)
	if err != nil || acc == nil || acc.Banned {
		return "free"
	}
	if acc.Admin || acc.Agent {
		return "pro"
	}
	if m := Monthly(owner); m.Credits > 0 {
		return m.Tier
	}
	return "free"
}

func init() { auth.SubscriptionTier = Tier }
