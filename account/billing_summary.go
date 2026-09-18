package account

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
)

func billingSummary(acc *auth.Account) string {
	body := `<p><strong>` + thousands(Balance(acc.ID)) + ` credits</strong></p><div class="form-actions">`
	if TopUpConfigured() {
		body += `<a class="btn" href="/account/topup">Top up</a>`
	}
	body += `<a class="btn" href="/account/transfer">Transfer</a><a href="/account/usage">Usage</a><a href="/account/billing">Transactions</a></div>`
	if PaymentsEnabled() && !acc.Admin && !acc.Agent && quota.DailyCredits() > 0 {
		body += `<p>Daily quota: ` + thousands(IncludedToday(acc.ID)) + ` / ` + thousands(quota.DailyCredits()) + ` credits remaining · resets 00:00 UTC</p>`
	}
	return app.SectionID("balance", "Balance", body)
}
