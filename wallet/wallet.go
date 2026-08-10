// Package wallet is money: what this instance charges, and how it gets paid.
//
// It is a staple rather than a service, which is why it sits at the top level
// beside home, agent, client and admin instead of under service/. It was under
// service/ and that put it in the wrong place twice over — as one entry in the
// catalogue of what this instance offers, and as a leaf that internal/api,
// internal/server and fifteen sibling services all had to import in order to
// ask what something cost.
//
// They do not ask here any more. The price list and the gate in front of it are
// internal/quota, which knows what things cost and deliberately does not know
// what a balance is; this package fills in the half quota cannot answer, from
// its own init, because quota sits underneath it and there is no cycle to
// break. A service imports quota. Nothing but the server, the two aggregating
// surfaces and this package's own page imports the wallet.
//
// The name is the caller's word, not an accountant's. On the x402 path there is
// no account, no credit balance and no invoice — a wallet signs, and that is
// the whole of it. Credits are the other rail into the same meter.
package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"mu/internal/quota"

	"mu/internal/auth"
	"mu/internal/data"

	"github.com/google/uuid"
)

const DailyTransferCap = 10000

// PaymentsEnabled returns true if payments are configured
// When false, quotas are disabled (self-hosted, no restrictions)
func PaymentsEnabled() bool {
	return StripeEnabled() || X402Enabled()
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
var wallets = map[string]*Wallet{}
var transactions = map[string][]*Transaction{}
var dailyUsage = map[string]*DailyUsage{}

// Wallet represents a user's credit balance
type Wallet struct {
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
	quota.Balance = GetBalance
	quota.Record = RecordUsage
	quota.Deduct = func(account, operation string, amount int, meta map[string]interface{}) error {
		return DeductCredits(account, amount, operation, meta)
	}

	// Load wallets from disk
	b, _ := data.LoadFile("wallets.json")
	json.Unmarshal(b, &wallets)

	// Load transactions from disk
	b, _ = data.LoadFile("transactions.json")
	json.Unmarshal(b, &transactions)

	// Load daily usage from disk
	b, _ = data.LoadFile("daily_usage.json")
	json.Unmarshal(b, &dailyUsage)
}

// Load initializes wallet
func Load() {
	// Wallet loaded
}

// GetWallet retrieves or creates a wallet for a user
func GetWallet(userID string) *Wallet {
	mutex.RLock()
	w, exists := wallets[userID]
	mutex.RUnlock()

	if !exists {
		w = &Wallet{
			UserID:    userID,
			Balance:   0,
			Currency:  "GBP",
			UpdatedAt: time.Now(),
		}
		mutex.Lock()
		wallets[userID] = w
		data.SaveJSON("wallets.json", wallets)
		mutex.Unlock()
	}

	return w
}

// GetBalance returns the current balance for a user
func GetBalance(userID string) int {
	w := GetWallet(userID)
	return w.Balance
}

// AddCredits adds credits to a user's wallet
func AddCredits(userID string, amount int, operation string, metadata map[string]interface{}) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	mutex.Lock()
	defer mutex.Unlock()

	w, exists := wallets[userID]
	if !exists {
		w = &Wallet{
			UserID:   userID,
			Balance:  0,
			Currency: "GBP",
		}
		wallets[userID] = w
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
	data.SaveJSON("wallets.json", wallets)
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

	w, exists := wallets[userID]
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
	data.SaveJSON("wallets.json", wallets)
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
	sender, exists := wallets[fromUserID]
	if !exists || sender.Balance < amount {
		return errors.New("insufficient credits")
	}

	// Get or create receiver wallet
	receiver, exists := wallets[toUserID]
	if !exists {
		receiver = &Wallet{
			UserID:   toUserID,
			Balance:  0,
			Currency: "GBP",
		}
		wallets[toUserID] = receiver
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
		Metadata:  map[string]interface{}{"to": toUserID, "to_name": AccountLabel(toUserID)},
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
		Metadata:  map[string]interface{}{"from": fromUserID, "from_name": AccountLabel(fromUserID)},
		CreatedAt: now,
	}
	transactions[toUserID] = append(transactions[toUserID], receiverTx)

	// Persist
	data.SaveJSON("wallets.json", wallets)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// AccountLabel is the name to show for an account id, falling back to the id
// when there is no account to ask — a deleted one, or a wallet that outlived
// it. Never empty, so a receipt always says something.
func AccountLabel(id string) string {
	if acc, err := auth.GetAccount(id); err == nil && strings.TrimSpace(acc.Name) != "" {
		return acc.Name
	}
	return id
}

// GetTransactions returns transaction history for a user
func GetTransactions(userID string, limit int) []*Transaction {
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

	w, exists := wallets[userID]
	if !exists {
		w = &Wallet{
			UserID:    userID,
			Balance:   0,
			Currency:  "GBP",
			UpdatedAt: time.Now(),
		}
		wallets[userID] = w
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
	user, exists := wallets[userID]
	if !exists || user.Balance < price {
		return errors.New("insufficient credits")
	}

	// Get or create author wallet
	author, exists := wallets[authorID]
	if !exists {
		author = &Wallet{
			UserID:   authorID,
			Balance:  0,
			Currency: "GBP",
		}
		wallets[authorID] = author
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
	data.SaveJSON("wallets.json", wallets)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// FormatCredits formats credits as currency string
func FormatCredits(credits int) string {
	pounds := credits / 100
	pence := credits % 100
	return fmt.Sprintf("£%d.%02d", pounds, pence)
}

// DeleteWallet removes a user's wallet and transaction history.
func DeleteWallet(userID string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(wallets, userID)
	delete(transactions, userID)
	data.SaveJSON("wallets.json", wallets)
	data.SaveJSON("transactions.json", transactions)
}
