// Credits: what an account has to spend, and how it gets more.
//
// This is the second half of the account — the first is who you are, and this
// is what you can afford. They are one thing to a person and were two
// directories to us: a nav item called Wallet that held a balance, a card
// form and a ledger, sitting beside an Account page that held everything else
// about the same account. Nobody has a wallet here. They have a balance.
//
// It used to be a top-level staple called wallet/, and the name was doing two
// jobs at once — this ledger, and a key that signs. Splitting them left this
// half with no name of its own, which is the tell: it never had one. It is the
// account's money, so it lives with the account.
//
// The price list and the gate in front of it are internal/quota, which knows
// what things cost and deliberately does not know what a balance is. This file
// fills in the half quota cannot answer, from its own init, because quota sits
// underneath and there is no cycle to break.
//
// One meter, not two. A person tops up with a card and an agent holding their
// token spends the same credits; an agent with no account pays per request over
// x402 and never comes here at all.
package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"mu/internal/app"
	"strings"
	"sync"
	"time"

	"mu/internal/quota"

	"mu/internal/auth"
	"mu/internal/data"

	"github.com/google/uuid"

	"mu/internal/x402"
)

const DailyTransferCap = 10000

// PaymentsEnabled returns true if payments are configured
// When false, quotas are disabled (self-hosted, no restrictions)
func PaymentsEnabled() bool {
	return StripeEnabled() || x402.Enabled()
}

// Transaction types
const (
	TxTopup    = "topup"
	TxSpend    = "spend"
	TxRefund   = "refund"
	TxTransfer = "transfer"
)

// The ledger's lock, and the only thing that takes it.
//
// It was an RWMutex with every function locking by hand, and that shape is what
// produced the bug it eventually produced: CreditsOf took the read lock, let it
// go, and took the write lock — and a credit landing in that gap was
// overwritten. Nothing was wrong with any single line. The design invited the
// mistake, which is the kind that review does not catch twice.
//
// So: a plain Mutex, because there is no read lock left to upgrade from, and
// one function that takes it. Everything below the line is handed a *ledger,
// which cannot be made anywhere else — so a helper that touches balances or
// transactions cannot be called without the lock held, and it is checked by the
// compiler rather than by a comment saying "callers hold mutex".
var mutex sync.Mutex

// ledger is proof that the lock is held. It has no fields and needs none: its
// whole job is to be unforgeable outside withLedger.
type ledger struct{}

// withLedger runs fn with the ledger locked.
//
// The only place in this package that locks. TestTheLedgerHasOneDoor fails if
// another appears.
func withLedger[T any](fn func(*ledger) T) T {
	mutex.Lock()
	defer mutex.Unlock()
	return fn(&ledger{})
}

// Storage
var balances = map[string]*Credits{}
var transactions = map[string][]*Transaction{}
var dailyUsage = map[string]*DailyUsage{}

// Credits is what one account has to spend.
//
// The field names are the ones already on disk, and so is the filename. It is
// still wallets.json in a package with no wallet in it, because renaming a live
// money file is exactly how the last balance got destroyed: the code ships, the
// old file is never read again, and every account reads zero. Renaming it is a
// migration, not an edit. TestTheKeyStoreAndTheLedgerAreSeparate pins it.
type Credits struct {
	UserID    string    `json:"user_id"`
	Balance   int       `json:"balance"`  // Credits (1 credit = 1 cent = $0.01)
	Currency  string    `json:"currency"` // Always "USD"
	UpdatedAt time.Time `json:"updated_at"`
}

// Transaction represents a wallet transaction
type Transaction struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Type      string                 `json:"type"`      // "topup", "spend", "refund"
	Amount    int                    `json:"amount"`    // Positive for topup, negative for spend
	Balance   int                    `json:"balance"`   // Balance after transaction
	Operation string                 `json:"operation"` // e.g., "news_search", "topup"
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
}

// DailyUsage tracks quota used per day
type DailyUsage struct {
	UserID string `json:"user_id"`
	Date   string `json:"date"` // "2006-01-02" format
	Used   int    `json:"used"` // Quota used today
}

func init() {
	// The gate in front of every priced call is internal/quota, which knows what
	// things cost and deliberately does not know what a balance is. This is the
	// half it cannot answer.
	//
	// Installed here rather than by the server because this is a plain downward
	// import — quota sits underneath the wallet and cannot see it — so there is
	// no cycle to break and nothing for main to arbitrate. It also means any
	// build that links the wallet has a working meter, rather than one that
	// lets everything through because a hook stayed nil.
	quota.Enabled = PaymentsEnabled
	quota.Balance = Balance
	quota.Record = RecordUsage
	quota.Deduct = func(account, operation string, amount int, meta map[string]interface{}) error {
		return DeductCredits(account, amount, operation, meta)
	}

	// Load balances from disk, keeping only the ones that decoded into something.
	//
	// The key store used to write this same filename, and its records parse
	// cleanly into a Credits record: the field names simply do not match, so every
	// account arrives with no id and a balance of zero. That map is not empty —
	// it has an entry per account — so anything asking "does this account have a
	// wallet" gets yes, and the rebuild below skips every one of them.
	b, _ := data.LoadFile("wallets.json")
	json.Unmarshal(b, &balances)
	for id, w := range balances {
		if w == nil || (w.UserID == "" && w.Balance == 0) {
			delete(balances, id)
		}
	}

	// Load transactions from disk
	b, _ = data.LoadFile("transactions.json")
	json.Unmarshal(b, &transactions)

	// Load daily usage from disk
	b, _ = data.LoadFile("daily_usage.json")
	json.Unmarshal(b, &dailyUsage)

	rebuildFromTransactions()
}

// rebuildFromTransactions restores balances the ledger file no longer has.
//
// wallets.json was written by two different maps — this one and the key store
// in basewallet.go, which used the same filename — so whichever saved last
// destroyed the other. Accounts came back with a balance of zero having done
// nothing to spend it.
//
// Every transaction records the balance after it, so the ledger is
// reconstructable from a file neither writer ever touched. Only for accounts
// with no entry at all: an entry that survived is authoritative, and a balance
// somebody topped up after the last recorded transaction must not be rolled
// back to it.
func rebuildFromTransactions() {
	if len(transactions) == 0 {
		return
	}
	// Keyed by account already, but the newest row is what carries the balance
	// and the slice is not guaranteed to be in order.
	latest := map[string]*Transaction{}
	for id, list := range transactions {
		for _, t := range list {
			if t == nil {
				continue
			}
			if prev, ok := latest[id]; !ok || t.CreatedAt.After(prev.CreatedAt) {
				latest[id] = t
			}
		}
	}

	restored := 0
	for id, t := range latest {
		if _, ok := balances[id]; ok {
			continue
		}
		if t.Balance <= 0 {
			continue
		}
		balances[id] = &Credits{
			UserID: id, Balance: t.Balance, Currency: "USD", UpdatedAt: t.CreatedAt,
		}
		restored++
	}
	if restored > 0 {
		app.Log("wallet", "restored %d credit balances from the transaction log", restored)
		data.SaveJSON("wallets.json", balances) //nolint:errcheck
	}
}

// Load initializes wallet
func Load() {
	// Balances loaded
}

// CreditsOf retrieves or creates a wallet for a user
// CreditsOf returns what an account has, creating the record on first ask.
//
// A copy, not the record. Handing out the pointer let a caller read a balance
// while a credit was being added to it — an unsynchronised read of an int that
// the race detector calls what it is, and that the Go memory model gives no
// answer for at all. Nothing outside this file needs to write a balance, and
// everything that does write one goes through a function here.
//
// The check and the create are one critical section, which they were not.
// CreditsOf read the map under a read lock, released it, built an empty record
// and then stored it under the write lock — so a credit arriving in that gap
// created the account first and was then overwritten by the empty record.
// Somebody opening /account while their top-up settled lost the top-up. It is
// not a narrow window either: the test that found this hit it on the second
// attempt.
func CreditsOf(userID string) *Credits {
	return withLedger(func(l *ledger) *Credits {
		out := *creditsOf(l, userID)
		return &out
	})
}

// creditsOf returns the live record, creating it if there is none.
//
// Takes a *ledger, so it cannot be reached without the lock. Must not let the
// pointer escape.
func creditsOf(l *ledger, userID string) *Credits {
	if w, exists := balances[userID]; exists && w != nil {
		return w
	}
	w := &Credits{
		UserID:    userID,
		Balance:   0,
		Currency:  "USD",
		UpdatedAt: time.Now(),
	}
	balances[userID] = w
	data.SaveJSON("wallets.json", balances) //nolint:errcheck
	return w
}

// Balance is what an account has to spend.
//
// A read, and only a read. It went through creditsOf, which creates the record
// when there is none and writes the whole of wallets.json to do it — so asking
// what somebody's balance was wrote a file. /admin/users asks it once per row,
// which on an instance with a few hundred accounts is a few hundred whole-file
// writes, serialised behind the ledger lock, on the way to drawing a table.
// An account with no row has no balance to report and the answer is zero;
// there is nothing to record about that.
func Balance(userID string) int {
	return withLedger(func(l *ledger) int {
		if w := balances[userID]; w != nil {
			return w.Balance
		}
		return 0
	})
}

// SettlementKey is the metadata field a one-time credit is deduped on.
//
// The value is the payment provider's own id for the purchase. It is spelled
// session_id because that is what every top-up already on disk calls it, and
// the point of this is that history counts: renaming it would make every
// transaction ever written invisible to the check below, which is the same
// mistake as renaming a live data file.
const SettlementKey = "session_id"

// CreditOnce credits an account unless the ledger already records this key.
//
// A payment can arrive twice — Stripe's webhook and the customer's return from
// checkout both settle, deliberately, so that a misconfigured webhook does not
// mean a charged card and nothing else. There was a map of seen session ids to
// stop the second one counting, and it was in memory: a restart emptied it, and
// Stripe retries a delivery for three days. Charge, settle, deploy, retry,
// credited twice.
//
// The fix is not a second file remembering what the first one did. Two stores
// that have to agree is the shape of the bug that destroyed this ledger once
// already. The ledger is the record: a top-up writes the session id into its
// own transaction, so asking whether a payment has settled is asking the only
// thing that knows. It survives restarts because it is the thing being
// restarted, it cannot drift from what was actually credited, and it answers
// correctly for every payment made before this was written.
//
// Returns whether it credited. Not an error when it did not — a payment that
// has already settled is the expected outcome of the second route arriving,
// not a failure.
func CreditOnce(userID string, amount int, operation, key string, metadata map[string]interface{}) (bool, error) {
	if strings.TrimSpace(key) == "" {
		return false, errors.New("a one-time credit needs a key to settle against")
	}
	if amount <= 0 {
		return false, errors.New("amount must be positive")
	}

	// The check and the write are one critical section. Two deliveries racing
	// would otherwise both look up, both find nothing, and both credit.
	var credited bool
	err := withLedger(func(l *ledger) error {
		if settled(l, userID, key) {
			return nil
		}
		if metadata == nil {
			metadata = map[string]interface{}{}
		}
		metadata[SettlementKey] = key
		credited = true
		return addCredits(l, userID, amount, operation, metadata)
	})
	return credited, err
}

// settled reports whether this account has already been credited for key.
func settled(l *ledger, userID, key string) bool {
	for _, tx := range transactions[userID] {
		if tx == nil || tx.Type != TxTopup {
			continue
		}
		if v, ok := tx.Metadata[SettlementKey].(string); ok && v == key {
			return true
		}
	}
	return false
}

// AddCredits adds credits to a user's wallet
func AddCredits(userID string, amount int, operation string, metadata map[string]interface{}) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	return withLedger(func(l *ledger) error {
		return addCredits(l, userID, amount, operation, metadata)
	})
}

// addCredits is AddCredits behind the door, so that a caller needing to decide
// and then write — CreditOnce — can do both without a gap.
func addCredits(l *ledger, userID string, amount int, operation string, metadata map[string]interface{}) error {
	w, exists := balances[userID]
	if !exists {
		w = &Credits{
			UserID:   userID,
			Balance:  0,
			Currency: "USD",
		}
		balances[userID] = w
	}

	w.Balance += amount
	w.UpdatedAt = time.Now()

	// Record transaction
	tx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      TxTopup,
		Amount:    amount,
		Balance:   w.Balance,
		Operation: operation,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}
	transactions[userID] = append(transactions[userID], tx)

	// Persist
	data.SaveJSON("wallets.json", balances)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// DeductCredits removes credits from a user's wallet
func DeductCredits(userID string, amount int, operation string, metadata map[string]interface{}) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	return withLedger(func(l *ledger) error {
		return deductCredits(l, userID, amount, operation, metadata)
	})
}

// deductCredits is DeductCredits with the lock held.
func deductCredits(l *ledger, userID string, amount int, operation string, metadata map[string]interface{}) error {

	w, exists := balances[userID]
	if !exists || w.Balance < amount {
		return errors.New("insufficient credits")
	}

	w.Balance -= amount
	w.UpdatedAt = time.Now()

	// Record transaction
	tx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      TxSpend,
		Amount:    -amount,
		Balance:   w.Balance,
		Operation: operation,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}
	transactions[userID] = append(transactions[userID], tx)

	// Persist
	data.SaveJSON("wallets.json", balances)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// DailyTransferTotal returns the number of credits transferred out by a user on the given UTC date.
func DailyTransferTotal(userID string, day time.Time) int {
	date := day.UTC().Format("2006-01-02")
	return withLedger(func(l *ledger) int { return transferredOn(l, userID, date) })
}

// transferredOn totals what an account sent on one UTC date.
//
// Shared with TransferCredits, which needs the same number while holding the
// lock it is about to write under. It was copied out into that function to
// avoid locking twice, and two copies of a cap is how one of them stops being
// the cap.
func transferredOn(l *ledger, userID, date string) int {
	total := 0
	for _, tx := range transactions[userID] {
		if tx == nil || tx.Type != TxTransfer || tx.Operation != quota.OpTransfer || tx.Amount >= 0 {
			continue
		}
		if tx.CreatedAt.UTC().Format("2006-01-02") != date {
			continue
		}
		total += -tx.Amount
	}
	return total
}

// TransferCredits transfers credits from one user to another
func TransferCredits(fromUserID, toUserID string, amount int) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}
	if fromUserID == toUserID {
		return errors.New("cannot transfer to yourself")
	}

	return withLedger(func(l *ledger) error {
		return transferCredits(l, fromUserID, toUserID, amount)
	})
}

// transferCredits is TransferCredits with the lock held.
func transferCredits(l *ledger, fromUserID, toUserID string, amount int) error {

	// The same total DailyTransferTotal reports, from the same function. It was
	// written out again here to avoid taking the lock twice, and a cap with two
	// implementations is a cap that one day only one of them enforces.
	today := time.Now().UTC().Format("2006-01-02")
	dailyTotal := transferredOn(l, fromUserID, today)
	if dailyTotal+amount > DailyTransferCap {
		return fmt.Errorf("daily transfer cap exceeded: maximum %d credits per day", DailyTransferCap)
	}

	// Check sender has sufficient balance
	sender, exists := balances[fromUserID]
	if !exists || sender.Balance < amount {
		return errors.New("insufficient credits")
	}

	// Get or create receiver wallet
	receiver, exists := balances[toUserID]
	if !exists {
		receiver = &Credits{
			UserID:   toUserID,
			Balance:  0,
			Currency: "USD",
		}
		balances[toUserID] = receiver
	}

	// Deduct from sender
	sender.Balance -= amount
	sender.UpdatedAt = time.Now()

	// Credit receiver
	receiver.Balance += amount
	receiver.UpdatedAt = time.Now()

	now := time.Now()
	txID := uuid.New().String()

	// Record sender transaction
	senderTx := &Transaction{
		ID:        txID,
		UserID:    fromUserID,
		Type:      TxTransfer,
		Amount:    -amount,
		Balance:   sender.Balance,
		Operation: quota.OpTransfer,
		// Both, because they answer different questions. The id is what was
		// actually credited and the only thing that can be reconciled; the
		// name is what the person typed and the only thing they can recognise.
		// Recording the id alone produced receipts reading "Transfer to 3834",
		// which is unreadable and — when the recipient was resolved wrongly —
		// indistinguishable from a correct one.
		Metadata:  map[string]interface{}{"to": toUserID, "to_name": Label(toUserID)},
		CreatedAt: now,
	}
	transactions[fromUserID] = append(transactions[fromUserID], senderTx)

	// Record receiver transaction
	receiverTx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    toUserID,
		Type:      TxTransfer,
		Amount:    amount,
		Balance:   receiver.Balance,
		Operation: quota.OpTransfer,
		Metadata:  map[string]interface{}{"from": fromUserID, "from_name": Label(fromUserID)},
		CreatedAt: now,
	}
	transactions[toUserID] = append(transactions[toUserID], receiverTx)

	// Persist
	data.SaveJSON("wallets.json", balances)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// Label is the name to show for an account id, falling back to the id
// when there is no account to ask — a deleted one, or a wallet that outlived
// it. Never empty, so a receipt always says something.
func Label(id string) string {
	if acc, err := auth.GetAccount(id); err == nil && strings.TrimSpace(acc.Name) != "" {
		return acc.Name
	}
	return id
}

// Transactions returns transaction history for a user
func Transactions(userID string, limit int) []*Transaction {
	return withLedger(func(_ *ledger) []*Transaction {
		txs := transactions[userID]
		if txs == nil {
			return []*Transaction{}
		}
		// Most recent first.
		result := make([]*Transaction, 0, len(txs))
		for i := len(txs) - 1; i >= 0 && len(result) < limit; i-- {
			result = append(result, txs[i])
		}
		return result
	})
}

// RecordUsage records a zero-cost usage transaction (for admins and quota tracking)
func RecordUsage(userID string, operation string) {
	withLedger(func(l *ledger) error {
		recordUsage(l, userID, operation)
		return nil
	})
}

// recordUsage is RecordUsage with the lock held.
func recordUsage(l *ledger, userID string, operation string) {

	w, exists := balances[userID]
	if !exists {
		w = &Credits{
			UserID:    userID,
			Balance:   0,
			Currency:  "USD",
			UpdatedAt: time.Now(),
		}
		balances[userID] = w
	}

	tx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      TxSpend,
		Amount:    0,
		Balance:   w.Balance,
		Operation: operation,
		CreatedAt: time.Now(),
	}
	transactions[userID] = append(transactions[userID], tx)
	data.SaveJSON("transactions.json", transactions)
}

// ChargeAppUse charges a user for using a paid app and pays the author.
// Returns error if user has insufficient credits. Author gets 70%, platform gets 30%.
func ChargeAppUse(userID, authorID, appSlug string, price int) error {
	if price <= 0 {
		return nil // Free app
	}
	if userID == authorID {
		return nil // Authors don't pay for their own apps
	}

	return withLedger(func(l *ledger) error {
		return chargeAppUse(l, userID, authorID, appSlug, price)
	})
}

// chargeAppUse is ChargeAppUse with the lock held.
func chargeAppUse(l *ledger, userID, authorID, appSlug string, price int) error {

	// Check sender has sufficient balance
	user, exists := balances[userID]
	if !exists || user.Balance < price {
		return errors.New("insufficient credits")
	}

	// Get or create author wallet
	author, exists := balances[authorID]
	if !exists {
		author = &Credits{
			UserID:   authorID,
			Balance:  0,
			Currency: "USD",
		}
		balances[authorID] = author
	}

	// Calculate split: author gets 90%, platform gets 10%
	authorShare := (price * 90) / 100
	if authorShare < 1 && price > 0 {
		authorShare = 1 // Minimum 1 credit to author
	}

	// Deduct from user
	user.Balance -= price
	user.UpdatedAt = time.Now()

	// Credit author
	author.Balance += authorShare
	author.UpdatedAt = time.Now()

	now := time.Now()

	// Record user spend
	userTx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      TxSpend,
		Amount:    -price,
		Balance:   user.Balance,
		Operation: quota.OpAppUse,
		Metadata:  map[string]interface{}{"app": appSlug, "author": authorID},
		CreatedAt: now,
	}
	transactions[userID] = append(transactions[userID], userTx)

	// Record author revenue
	authorTx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    authorID,
		Type:      TxTopup,
		Amount:    authorShare,
		Balance:   author.Balance,
		Operation: quota.OpAppRevenue,
		Metadata:  map[string]interface{}{"app": appSlug, "from": userID, "price": price},
		CreatedAt: now,
	}
	transactions[authorID] = append(transactions[authorID], authorTx)

	// Persist
	data.SaveJSON("wallets.json", balances)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// FormatCredits formats credits as currency string
func FormatCredits(credits int) string {
	dollars := credits / 100
	cents := credits % 100
	return fmt.Sprintf("$%d.%02d", dollars, cents)
}

// DeleteCredits removes a user's wallet and transaction history.
func DeleteCredits(userID string) {
	withLedger(func(l *ledger) error {
		deleteCredits(l, userID)
		return nil
	})
}

// deleteCredits is DeleteCredits with the lock held.
func deleteCredits(l *ledger, userID string) {
	delete(balances, userID)
	delete(transactions, userID)
	data.SaveJSON("wallets.json", balances)
	data.SaveJSON("transactions.json", transactions)
}
