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
	return StripeEnabled() || x402.X402Enabled()
}

// Transaction types
const (
	TxTopup    = "topup"
	TxSpend    = "spend"
	TxRefund   = "refund"
	TxTransfer = "transfer"
)

var mutex sync.RWMutex

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
	Balance   int       `json:"balance"`  // Credits (1 credit = 1 penny = £0.01)
	Currency  string    `json:"currency"` // Always "GBP" for now
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
			UserID: id, Balance: t.Balance, Currency: "GBP", UpdatedAt: t.CreatedAt,
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
func CreditsOf(userID string) *Credits {
	mutex.RLock()
	w, exists := balances[userID]
	mutex.RUnlock()

	if !exists {
		w = &Credits{
			UserID:    userID,
			Balance:   0,
			Currency:  "GBP",
			UpdatedAt: time.Now(),
		}
		mutex.Lock()
		balances[userID] = w
		data.SaveJSON("wallets.json", balances)
		mutex.Unlock()
	}

	return w
}

// Balance returns the current balance for a user
func Balance(userID string) int {
	w := CreditsOf(userID)
	return w.Balance
}

// AddCredits adds credits to a user's wallet
func AddCredits(userID string, amount int, operation string, metadata map[string]interface{}) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	mutex.Lock()
	defer mutex.Unlock()

	w, exists := balances[userID]
	if !exists {
		w = &Credits{
			UserID:   userID,
			Balance:  0,
			Currency: "GBP",
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

	mutex.Lock()
	defer mutex.Unlock()

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

	mutex.RLock()
	defer mutex.RUnlock()

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

	mutex.Lock()
	defer mutex.Unlock()

	// Check sender has not exceeded the daily transfer cap.
	today := time.Now().UTC().Format("2006-01-02")
	dailyTotal := 0
	for _, tx := range transactions[fromUserID] {
		if tx == nil || tx.Type != TxTransfer || tx.Operation != quota.OpTransfer || tx.Amount >= 0 {
			continue
		}
		if tx.CreatedAt.UTC().Format("2006-01-02") == today {
			dailyTotal += -tx.Amount
		}
	}
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
			Currency: "GBP",
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
	mutex.RLock()
	defer mutex.RUnlock()

	txs := transactions[userID]
	if txs == nil {
		return []*Transaction{}
	}

	// Return most recent first
	result := make([]*Transaction, 0, len(txs))
	for i := len(txs) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, txs[i])
	}
	return result
}

// RecordUsage records a zero-cost usage transaction (for admins and quota tracking)
func RecordUsage(userID string, operation string) {
	mutex.Lock()
	defer mutex.Unlock()

	w, exists := balances[userID]
	if !exists {
		w = &Credits{
			UserID:    userID,
			Balance:   0,
			Currency:  "GBP",
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

	mutex.Lock()
	defer mutex.Unlock()

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
			Currency: "GBP",
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
	pounds := credits / 100
	pence := credits % 100
	return fmt.Sprintf("£%d.%02d", pounds, pence)
}

// DeleteCredits removes a user's wallet and transaction history.
func DeleteCredits(userID string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(balances, userID)
	delete(transactions, userID)
	data.SaveJSON("wallets.json", balances)
	data.SaveJSON("transactions.json", transactions)
}
