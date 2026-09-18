package account

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
)

// billingSummary is shared by people using the web app and the API.
func billingSummary(acc *auth.Account) string {
	body := `<p>The app and API use the same allowance and credit balance.</p>`
	if !PaymentsEnabled() {
		body += `<p>Usage is not charged on this instance.</p>`
	} else if acc.Admin || acc.Agent {
		body += `<p>This account is exempt from usage charges.</p>`
	} else if daily := quota.DailyCredits(); daily > 0 {
		body += `<p>Daily allowance: <strong>` + thousands(IncludedToday(acc.ID)) + ` of ` + thousands(daily) + ` credits left</strong>. Renews at 00:00 UTC.</p>`
		if cap := quota.DailyPoolCredits(); cap > 0 && IncludedUsage() >= cap {
			body += `<p>The shared free allowance is used up for today. Your prepaid credit is still available.</p>`
		}
	}
	body += `<p>Credit balance: <strong>` + thousands(CreditsOf(acc.ID).Balance) + ` credits</strong></p>`
	if PaymentsEnabled() {
		body += `<p>Included credit is used first, then prepaid credit. There is no subscription currently.</p>`
	}
	body += `<div class="form-actions">`
	if TopUpConfigured() {
		body += `<a href="/account/topup">Add credit</a>`
	}
	body += `<a href="/pricing">Pricing</a></div>`
	body += `<details class="disclosure"><summary>Credit transfers</summary><p><a href="/account/transfer">Transfer credit to another account</a></p></details>`
	return app.Section("Balance", body)
}
