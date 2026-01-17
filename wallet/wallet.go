package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"mu/auth"
	"mu/data"

	"github.com/google/uuid"
)

// Credit costs per operation (in credits/pennies)
var (
	CostNewsSearch    = getEnvInt("CREDIT_COST_NEWS", 1)
	CostNewsSummary   = getEnvInt("CREDIT_COST_NEWS_SUMMARY", 1)
	CostVideoSearch   = getEnvInt("CREDIT_COST_VIDEO", 2)
	CostVideoWatch    = getEnvInt("CREDIT_COST_VIDEO_WATCH", 2)
	CostChatQuery     = getEnvInt("CREDIT_COST_CHAT", 3)
	CostChatRoom      = getEnvInt("CREDIT_COST_CHAT_ROOM", 1)
	CostAppCreate     = getEnvInt("CREDIT_COST_APP_CREATE", 5)
	CostAppModify     = getEnvInt("CREDIT_COST_APP_MODIFY", 3)
	CostAgentRun      = getEnvInt("CREDIT_COST_AGENT", 5)
	FreeDailySearches = getEnvInt("FREE_DAILY_SEARCHES", 10)
)

// Operation types
const (
	OpNewsSearch  = "news_search"
	OpNewsSummary = "news_summary"
	OpVideoSearch = "video_search"
	OpVideoWatch  = "video_watch"
	OpChatQuery   = "chat_query"
	OpChatRoom    = "chat_room"
	OpAppCreate   = "app_create"
	OpAppModify   = "app_modify"
	OpAgentRun    = "agent_run"
	OpTopup       = "topup"
	OpRefund      = "refund"
)

// Transaction types
const (
	TxTopup  = "topup"
	TxSpend  = "spend"
	TxRefund = "refund"
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

// DailyUsage tracks free searches used per day
type DailyUsage struct {
	UserID   string `json:"user_id"`
	Date     string `json:"date"`     // "2006-01-02" format
	Searches int    `json:"searches"` // Free searches used today
}

// TopupTier represents a credit purchase option
type TopupTier struct {
	Amount   int    `json:"amount"`    // Price in pence (e.g., 500 = £5)
	Credits  int    `json:"credits"`   // Credits received
	BonusPct int    `json:"bonus_pct"` // Bonus percentage
	PriceID  string `json:"price_id"`  // Stripe price ID
}

// Available topup tiers
var TopupTiers = []TopupTier{
	{Amount: 500, Credits: 500, BonusPct: 0},
	{Amount: 1000, Credits: 1050, BonusPct: 5},
	{Amount: 2500, Credits: 2750, BonusPct: 10},
	{Amount: 5000, Credits: 5750, BonusPct: 15},
}

func init() {
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

// getEnvInt gets an environment variable as int with default
func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		fmt.Sscanf(v, "%d", &i)
		if i > 0 {
			return i
		}
	}
	return defaultVal
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

// GetDailyUsage gets or creates daily usage record
func GetDailyUsage(userID string) *DailyUsage {
	today := time.Now().UTC().Format("2006-01-02")
	key := userID + ":" + today

	mutex.RLock()
	usage, exists := dailyUsage[key]
	mutex.RUnlock()

	if !exists || usage.Date != today {
		usage = &DailyUsage{
			UserID:   userID,
			Date:     today,
			Searches: 0,
		}
		mutex.Lock()
		dailyUsage[key] = usage
		// Clean up old entries (keep only today)
		for k, v := range dailyUsage {
			if v.Date != today {
				delete(dailyUsage, k)
			}
		}
		data.SaveJSON("daily_usage.json", dailyUsage)
		mutex.Unlock()
	}

	return usage
}

// HasFreeSearches checks if user has free searches remaining today
func HasFreeSearches(userID string) bool {
	usage := GetDailyUsage(userID)
	return usage.Searches < FreeDailySearches
}

// GetFreeSearchesRemaining returns remaining free searches for today
func GetFreeSearchesRemaining(userID string) int {
	usage := GetDailyUsage(userID)
	remaining := FreeDailySearches - usage.Searches
	if remaining < 0 {
		return 0
	}
	return remaining
}

// UseFreeSearch consumes one free search
func UseFreeSearch(userID string) error {
	today := time.Now().UTC().Format("2006-01-02")
	key := userID + ":" + today

	mutex.Lock()
	defer mutex.Unlock()

	usage, exists := dailyUsage[key]
	if !exists || usage.Date != today {
		usage = &DailyUsage{
			UserID:   userID,
			Date:     today,
			Searches: 0,
		}
		dailyUsage[key] = usage
	}

	if usage.Searches >= FreeDailySearches {
		return errors.New("daily free searches exhausted")
	}

	usage.Searches++
	data.SaveJSON("daily_usage.json", dailyUsage)

	return nil
}

// GetOperationCost returns the credit cost for an operation
func GetOperationCost(operation string) int {
	switch operation {
	case OpNewsSearch:
		return CostNewsSearch
	case OpNewsSummary:
		return CostNewsSummary
	case OpVideoSearch:
		return CostVideoSearch
	case OpVideoWatch:
		return CostVideoWatch
	case OpChatQuery:
		return CostChatQuery
	case OpChatRoom:
		return CostChatRoom
	case OpAppCreate:
		return CostAppCreate
	case OpAppModify:
		return CostAppModify
	case OpAgentRun:
		return CostAgentRun
	default:
		return 1
	}
}

// CheckQuota checks if a user can perform an operation
// Returns: canProceed, useFreeSearch, creditCost, error
func CheckQuota(userID string, operation string) (bool, bool, int, error) {
	// Get account to check member/admin status
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return false, false, 0, errors.New("account not found")
	}

	// Members and admins have unlimited access
	if acc.Member || acc.Admin {
		return true, false, 0, nil
	}

	cost := GetOperationCost(operation)

	// Check if user has free searches remaining
	if HasFreeSearches(userID) {
		return true, true, 0, nil
	}

	// Check if user has sufficient credits
	balance := GetBalance(userID)
	if balance >= cost {
		return true, false, cost, nil
	}

	// User needs to top up
	return false, false, cost, errors.New("insufficient credits")
}

// ConsumeQuota consumes quota for an operation (call after successful operation)
func ConsumeQuota(userID string, operation string) error {
	// Get account to check member/admin status
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return errors.New("account not found")
	}

	// Members and admins don't consume quota
	if acc.Member || acc.Admin {
		return nil
	}

	// Try free search first
	if HasFreeSearches(userID) {
		return UseFreeSearch(userID)
	}

	// Deduct credits
	cost := GetOperationCost(operation)
	return DeductCredits(userID, cost, operation, nil)
}

// FormatCredits formats credits as currency string
func FormatCredits(credits int) string {
	pounds := credits / 100
	pence := credits % 100
	return fmt.Sprintf("£%d.%02d", pounds, pence)
}

// GetTopupTier returns the topup tier for a given amount
func GetTopupTier(amount int) *TopupTier {
	for i := range TopupTiers {
		if TopupTiers[i].Amount == amount {
			return &TopupTiers[i]
		}
	}
	return nil
}
