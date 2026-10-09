package gateway

import (
	"fmt"
	"mu/internal/abuse"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/service/wallet"
	"mu/x402/payment"
	"net/http"
	"time"
)

// Configure installs the shared payment policy after runtime initialization.
func Configure() {
	service.Gate.Allow = func(account, op string) (bool, error) {
		if account == "" {
			if quota.OperationCost(op) > 0 {
				return false, fmt.Errorf("authentication or verified payment required for paid operations")
			}
			return false, nil
		}

		if quota.OperationCost(op) > 0 {
			if !api.IsWalletIdentity(account) {
				acc, err := auth.GetAccount(account)
				if err != nil || acc.Banned || (!acc.Agent && !acc.Admin && !acc.Approved && !acc.EmailVerified) {
					return false, fmt.Errorf("a verified or approved account is required")
				}
			}
			for _, b := range []struct {
				key    string
				max    int
				window time.Duration
			}{
				{"paid:hour:", abuse.Limit("PAID_MAX_PER_HOUR", 300), time.Hour},
				{"paid:day:", abuse.Limit("PAID_MAX_PER_DAY", 1000), 24 * time.Hour},
			} {
				wait, err := abuse.Take(b.key+account, b.max, b.window)
				if err != nil {
					return false, err
				}
				if wait > 0 {
					return false, fmt.Errorf("paid operation limit reached; retry in %d seconds", int(wait.Seconds()))
				}
			}
		}

		// Somebody who paid in USDC has already paid, at the door, for this
		// exact operation — VerifyAndSettle runs before the tool does, and the
		// free trial is counted there too. There is no account behind a wallet
		// identity by design: not signing up is the entire point of x402.
		//
		// So asking the wallet about one gets "account not found", and the
		// gateway turns that into a refusal — of a call that has been paid for.
		// The money is gone and the caller has nothing. This was live for every
		// scoped priced service, and binding the caller on priced tools spread
		// it to web_search, which is the example in the README.
		//
		// Allowed, recorded, and not charged again.
		if api.IsWalletIdentity(account) {
			quota.Record(account, op)
			return false, nil
		}

		// A daily limit is checked before anything about money, because it is
		// not about money. It is the second control: a price stops somebody who
		// has to pay and does nothing about a loop, and what a loop spends on
		// the three outbound operations is a domain's or a number's reputation,
		// which no balance repairs. See the limit block in quota.json.
		if over, why := quota.OverLimit(account, op); over {
			return false, fmt.Errorf("%s", why)
		}

		ok, _, cost, err := quota.CheckQuota(account, op)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, fmt.Errorf("%s", quota.Shortfall(cost, quota.Available(account)))
		}
		return true, nil
	}
	service.Gate.Reserve = quota.Reserve
	service.Gate.Charge = func(account, op string) {
		if err := quota.Charge(account, op, nil); err != nil {
			app.Log("wallet", "charging %s for %s: %v", account, op, err)
		}
	}
	// Served without reaching a paid provider: recorded, because /usage should
	// show what an account did, and not charged, because it cost nothing to
	// answer. See internal/service/meter.go.
	service.Gate.Free = func(account, op string) { quota.Record(account, op) }
	// One counter, moved once, after the call succeeded — it is what both the
	// free allowance and the daily limit read.
	service.Gate.Done = quota.Done

	api.QuotaCheck = Check
	api.WalletPayer = func(r *http.Request) string { return payment.PayerFrom(r.Context()) }
	api.WalletSigner = func(r *http.Request) string { return wallet.SignerFrom(r.Context()) }
}
