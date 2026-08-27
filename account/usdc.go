package account

// Turning USDC in your wallet into credits on this instance.
//
// The wallet page offered an address to send USDC to and nothing that could
// spend what arrived. Every path out of it was outbound — the CLI paying
// somebody else's priced endpoint — so money sent to your own address here
// bought you nothing here. A deposit box with no door.
//
// This is the door, and it is the same door Stripe uses. A payment arrives, the
// ledger is credited, and every meter downstream is unchanged: quota.Charge
// still deducts credits, prices are still in credits, usage still reads the
// same. x402 becomes a second way to top up rather than a second kind of money
// — which is the whole reason a credit is a cent now. USDC is dollars, a credit
// is a cent, so a hundred credits is one USDC and there is no rate in the
// middle to drift, quote or hedge.
//
// Mechanically it is this instance paying itself. The key is custodial and
// already here, so there is no browser wallet to prompt and no chain for
// somebody to pick wrong: sign an EIP-3009 authorisation from the account's own
// wallet to X402_PAY_TO, hand it to the facilitator to broadcast, and credit
// the ledger against the transaction hash it returns.

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"
	neturl "net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/x402"
	"mu/service/wallet"
)

// minConvertCredits is the smallest conversion worth making.
//
// A dollar. Not because anything breaks below it, but because every conversion
// is an on-chain transfer somebody pays for — the facilitator sponsors the gas
// and recovers it somewhere — and converting ten cents at a time is a way to
// spend more moving money than the money is worth.
const minConvertCredits = 100

// maxConvertCredits bounds a single conversion, matching the card top-up.
//
// The wallet is custodial: an operator who can read the disk can spend it, and
// this instance says so on the export page. A cap does not change that and is
// not pretending to — it bounds one mistake, not the trust.
const maxConvertCredits = maxTopupDollars * 100

// ConvertUSDC moves USDC from the caller's wallet into their credit balance.
func ConvertUSDC(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil || sess == nil || acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/wallet", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		convertFailed(w, r, "That form could not be read.")
		return
	}

	credits, err := convertAmount(r.FormValue("amount"))
	if err != nil {
		convertFailed(w, r, err.Error())
		return
	}

	added, tx, err := convertUSDC(acc.ID, credits)
	if err != nil {
		app.Log("wallet", "USDC conversion failed for %s: %v", acc.ID, err)
		convertFailed(w, r, err.Error())
		return
	}
	if !added {
		// The transaction settled and the ledger already had it. Not a failure:
		// it is what a retry looks like, and the balance is already right.
		http.Redirect(w, r, "/wallet?saved=converted", http.StatusSeeOther)
		return
	}

	app.Log("wallet", "converted %d credits of USDC for %s (tx %s)", credits, acc.ID, tx)
	http.Redirect(w, r, "/wallet?saved=converted", http.StatusSeeOther)
}

// convertAmount reads what somebody typed, in whole dollars.
//
// Dollars rather than credits, because the box sits under a balance shown in
// USDC and asking somebody to convert "500" of a thing they hold 5 of is a
// question about arithmetic rather than about money.
func convertAmount(s string) (int, error) {
	var dollars int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &dollars); err != nil || dollars < 1 {
		return 0, errors.New("Enter an amount in whole dollars.")
	}
	credits := dollars * 100
	if credits < minConvertCredits {
		return 0, errors.New("The smallest conversion is $1.")
	}
	if credits > maxConvertCredits {
		return 0, fmt.Errorf("The largest conversion is $%d.", maxConvertCredits/100)
	}
	return credits, nil
}

// convertUSDC is the conversion itself: sign, settle, credit.
//
// Credited against the transaction hash rather than against the attempt, so the
// ledger settles once per on-chain movement. A form resubmitted, a browser that
// retried, two tabs — all of them produce at most one credit for one transfer,
// which is the same promise CreditOnce makes to the Stripe webhook.
func convertUSDC(accountID string, credits int) (bool, string, error) {
	req := x402.TopUpRequirement(credits)
	if req == nil {
		return false, "", errors.New("This instance does not take USDC. Top up with a card instead.")
	}

	bw, err := wallet.EnsureFor(accountID)
	if err != nil || bw == nil || bw.Address == "" {
		return false, "", errors.New("Your wallet could not be opened, so there is nothing to convert from.")
	}

	// Asked before signing, so somebody converting more than they hold is told
	// so plainly rather than through a facilitator rejection. It is a check and
	// not a guarantee: the balance can change between here and settlement, and
	// the facilitator is what actually refuses.
	if _, atomic, err := wallet.USDCBalanceErr(bw.Address); err == nil {
		if want := creditsAsUSDCAtomic(credits); atomic != nil && want != nil && atomic.Cmp(want) < 0 {
			return false, "", fmt.Errorf("Your wallet holds less than $%d.", credits/100)
		}
	}

	hdr, err := wallet.SignX402Payment(bw, *req)
	if err != nil {
		return false, "", fmt.Errorf("the payment could not be signed: %w", err)
	}

	res, err := x402.SettleSigned(hdr, req)
	if err != nil {
		return false, "", fmt.Errorf("the transfer did not go through: %w", err)
	}
	tx := strings.TrimSpace(res.Transaction)
	if tx == "" {
		// Settled with no transaction to key on. Refusing to credit is the safe
		// side of this: a credit with no key is one that cannot be recognised as
		// already paid, so a retry would double it.
		return false, "", errors.New("the transfer settled without a transaction id, so it has not been credited — the server log under wallet has the detail")
	}

	added, err := CreditOnce(accountID, credits, "topup_usdc", tx, map[string]interface{}{
		"source":  "usdc",
		"tx":      tx,
		"network": res.Network,
		"from":    bw.Address,
	})
	if err != nil {
		return false, tx, fmt.Errorf("the transfer settled as %s but the credit failed: %w", tx, err)
	}
	return added, tx, nil
}

// creditsAsUSDCAtomic is what a credit total is worth in USDC's smallest unit.
//
// A credit is a cent and USDC carries six decimals, so a credit is 10,000 of
// them. Written out here rather than reaching into internal/x402 for the same
// sum: the requirement it builds is what the facilitator is actually paid, and
// this is only for telling somebody they are short before they sign.
func creditsAsUSDCAtomic(credits int) *big.Int {
	return new(big.Int).Mul(big.NewInt(int64(credits)), big.NewInt(10000))
}

// convertFailed puts the reason back on the wallet page.
func convertFailed(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/wallet?error="+neturl.QueryEscape(msg), http.StatusSeeOther)
}
