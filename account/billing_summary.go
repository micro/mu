package account

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
)

func billingSummary(acc *auth.Account) string {
	body := `<p><strong>` + thousands(Balance(acc.ID)) + ` prepaid credits</strong></p>`
	if remaining := SignupRemaining(acc.ID); remaining > 0 {
		body += `<p><strong>` + thousands(remaining) + ` signup credits</strong> · for usage only</p>`
	}
	body += `<div class="form-actions">`
	if TopUpConfigured() {
		body += `<a class="btn" href="/account/topup">Top up</a>`
	}
	body += `<a class="btn" href="/account/transfer">Transfer</a></div><p><a href="/account/usage">Usage</a></p>`
	if PaymentsEnabled() && !acc.Admin && !acc.Agent && quota.DailyCredits() > 0 {
		body += `<p>Daily quota: ` + thousands(IncludedToday(acc.ID)) + ` / ` + thousands(quota.DailyCredits()) + ` credits remaining · resets 00:00 UTC</p>`
	}
	return app.SectionID("balance", "Balance", body)
}
