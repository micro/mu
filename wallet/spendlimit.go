package wallet

// Spend limits for agent-initiated x402 payments. The agent spends the user's
// wallet on their behalf, so — even though the source account is always bound
// to the authenticated user — a misled or prompt-injected agent must not be
// able to drain it. Every payment is bounded per-call and per-day; a server's
// 402 challenge can never authorise more than these caps.

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"mu/internal/settings"
)

// Default caps in USDC atomic units (6 decimals): $1 per call, $10 per day.
// Override with X402_MAX_SPEND / X402_DAILY_SPEND (atomic units).
const (
	defaultMaxSpendPerCall = 1_000_000
	defaultMaxSpendPerDay  = 10_000_000
)

func maxSpendPerCall() *big.Int { return settingAtomic("X402_MAX_SPEND", defaultMaxSpendPerCall) }
func maxSpendPerDay() *big.Int  { return settingAtomic("X402_DAILY_SPEND", defaultMaxSpendPerDay) }

// settingAtomic reads a settings value as an atomic-unit integer, falling back
// to def when unset or unparseable.
func settingAtomic(key string, def int64) *big.Int {
	if v := settings.Get(key); v != "" {
		if n, ok := new(big.Int).SetString(v, 10); ok && n.Sign() > 0 {
			return n
		}
	}
	return big.NewInt(def)
}

var (
	spendMu     sync.Mutex
	spendByUser = map[string]*daySpend{} // accountID -> today's running total
)

type daySpend struct {
	day   string // YYYY-MM-DD (UTC)
	spent *big.Int
}

// checkAndRecordSpend authorises a payment of amount (atomic units) for
// accountID against the per-call and per-day caps, recording it on success.
// It records before settlement (fails safe: an aborted payment only makes the
// remaining budget stricter, never looser).
func checkAndRecordSpend(accountID string, amount *big.Int) error {
	if amount == nil || amount.Sign() <= 0 {
		return fmt.Errorf("invalid payment amount")
	}
	perCall := maxSpendPerCall()
	if amount.Cmp(perCall) > 0 {
		return fmt.Errorf("payment of %s exceeds the per-call limit of %s USDC atomic units", amount, perCall)
	}
	if accountID == "" {
		// No account to meter a daily total against; the per-call cap still applies.
		return nil
	}

	today := time.Now().UTC().Format("2006-01-02")
	perDay := maxSpendPerDay()

	spendMu.Lock()
	defer spendMu.Unlock()
	rec := spendByUser[accountID]
	if rec == nil || rec.day != today {
		rec = &daySpend{day: today, spent: big.NewInt(0)}
		spendByUser[accountID] = rec
	}
	next := new(big.Int).Add(rec.spent, amount)
	if next.Cmp(perDay) > 0 {
		return fmt.Errorf("payment would exceed the daily spend limit of %s USDC atomic units (already spent %s today)", perDay, rec.spent)
	}
	rec.spent = next
	return nil
}
