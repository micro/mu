package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"mu/internal/auth"
	"mu/internal/data"

	"github.com/google/uuid"
)

// Credit costs per operation (in credits/pennies)
// Read-only operations (news reading, blog reading, video watching, chat viewing) are included.
// Only actions that create content, trigger searches, or use external APIs are charged.
var (
	CostNewsSearch        = getEnvInt("CREDIT_COST_NEWS", 1)
	CostVideoSearch       = getEnvInt("CREDIT_COST_VIDEO", 2)
	CostChatQuery         = getEnvInt("CREDIT_COST_CHAT", 5)
	CostBlogCreate        = getEnvInt("CREDIT_COST_BLOG_CREATE", 1)
	CostMailSend          = getEnvInt("CREDIT_COST_MAIL", 1)  // Internal mail send
	CostExternalEmail     = getEnvInt("CREDIT_COST_EMAIL", 4) // External email (SMTP delivery cost)
	CostPlacesSearch      = getEnvInt("CREDIT_COST_PLACES_SEARCH", 5)
	CostPlacesNearby      = getEnvInt("CREDIT_COST_PLACES_NEARBY", 2)
	CostWeatherForecast   = getEnvInt("CREDIT_COST_WEATHER", 1)
	CostWeatherPollen     = getEnvInt("CREDIT_COST_WEATHER_POLLEN", 1)
	CostWebSearch         = getEnvInt("CREDIT_COST_SEARCH", 5)
	CostWebFetch          = getEnvInt("CREDIT_COST_FETCH", 3)
	CostAgentQuery        = getEnvInt("CREDIT_COST_AGENT", 3)
	CostAgentQueryPremium = getEnvInt("CREDIT_COST_AGENT_PREMIUM", 9)
	CostSocialSearch      = getEnvInt("CREDIT_COST_SOCIAL", 1)
	CostSocialPost        = getEnvInt("CREDIT_COST_SOCIAL_POST", 1)
	CostSocialReply       = getEnvInt("CREDIT_COST_SOCIAL_REPLY", 1)
	CostBlogComment       = getEnvInt("CREDIT_COST_BLOG_COMMENT", 1)
	CostAppBuild          = getEnvInt("CREDIT_COST_APP_BUILD", 100)
	CostAppEdit           = getEnvInt("CREDIT_COST_APP_EDIT", 50)
	DailyQuota            = getEnvInt("DAILY_QUOTA", getEnvInt("FREE_DAILY_QUOTA", 100))
)

// PaymentsEnabled returns true if payments are configured
// When false, quotas are disabled (self-hosted, no restrictions)
func PaymentsEnabled() bool {
	return StripeEnabled() || X402Enabled()
}

// Operation types
const (
	OpNewsSearch    = "news_search"
	OpVideoSearch   = "video_search"
	OpChatQuery     = "chat_query"
	OpBlogCreate    = "blog_create"
	OpMailSend      = "mail_send"
	OpExternalEmail = "external_email"
	OpPlacesSearch      = "places_search"
	OpPlacesNearby      = "places_nearby"
	OpWeatherForecast   = "weather_forecast"
	OpWeatherPollen     = "weather_pollen"
	OpWebSearch         = "web_search"
	OpWebFetch          = "web_fetch"
	OpAgentQuery        = "agent_query"
	OpAgentQueryPremium = "agent_query_premium"
	OpSocialSearch      = "social_search"
	OpSocialPost        = "social_post"
	OpSocialReply       = "social_reply"
	OpBlogComment       = "blog_comment"
	OpAppBuild          = "app_build"
	OpAppEdit           = "app_edit"
	OpAppUse            = "app_use"
	OpAppRevenue        = "app_revenue"
	OpTopup             = "topup"
	OpRefund            = "refund"
	OpTransfer          = "transfer"
	OpEscrowHold        = "escrow_hold"
	OpEscrowRelease     = "escrow_release"
	OpEscrowRefund      = "escrow_refund"
)

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
		Operation: OpTransfer,
		Metadata:  map[string]interface{}{"to": toUserID},
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
		Operation: OpTransfer,
		Metadata:  map[string]interface{}{"from": fromUserID},
		CreatedAt: now,
	}
	transactions[toUserID] = append(transactions[toUserID], receiverTx)

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
			UserID: userID,
			Date:   today,
			Used:   0,
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

// HasQuota checks if user has daily quota remaining
func HasQuota(userID string) bool {
	usage := GetDailyUsage(userID)
	return usage.Used < DailyQuota
}

// GetQuotaRemaining returns remaining daily quota
func GetQuotaRemaining(userID string) int {
	usage := GetDailyUsage(userID)
	remaining := DailyQuota - usage.Used
	if remaining < 0 {
		return 0
	}
	return remaining
}

// UseQuota consumes one daily quota unit
func UseQuota(userID string) error {
	today := time.Now().UTC().Format("2006-01-02")
	key := userID + ":" + today

	mutex.Lock()
	defer mutex.Unlock()

	usage, exists := dailyUsage[key]
	if !exists || usage.Date != today {
		usage = &DailyUsage{
			UserID: userID,
			Date:   today,
			Used:   0,
		}
		dailyUsage[key] = usage
	}

	if usage.Used >= DailyQuota {
		return errors.New("daily quota exhausted")
	}

	usage.Used++
	data.SaveJSON("daily_usage.json", dailyUsage)

	return nil
}

// GetOperationCost returns the credit cost for an operation
func GetOperationCost(operation string) int {
	switch operation {
	case OpNewsSearch:
		return CostNewsSearch
	case OpVideoSearch:
		return CostVideoSearch
	case OpChatQuery:
		return CostChatQuery
	case OpBlogCreate:
		return CostBlogCreate
	case OpMailSend:
		return CostMailSend
	case OpExternalEmail:
		return CostExternalEmail
	case OpPlacesSearch:
		return CostPlacesSearch
	case OpPlacesNearby:
		return CostPlacesNearby
	case OpWeatherForecast:
		return CostWeatherForecast
	case OpWeatherPollen:
		return CostWeatherPollen
	case OpWebSearch:
		return CostWebSearch
	case OpWebFetch:
		return CostWebFetch
	case OpAgentQuery:
		return CostAgentQuery
	case OpAgentQueryPremium:
		return CostAgentQueryPremium
	case OpSocialSearch:
		return CostSocialSearch
	case OpSocialPost:
		return CostSocialPost
	case OpSocialReply:
		return CostSocialReply
	case OpBlogComment:
		return CostBlogComment
	case OpAppBuild:
		return CostAppBuild
	case OpAppEdit:
		return CostAppEdit
	default:
		return 1
	}
}


// CheckQuota checks if a user can perform an operation
// Returns: canProceed, useQuota (always false now), creditCost, error
func CheckQuota(userID string, operation string) (bool, bool, int, error) {
	// Get account to check admin status
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return false, false, 0, errors.New("account not found")
	}

	// Admins have unlimited access
	if acc.Admin {
		return true, false, 0, nil
	}

	// If payments not configured, no quotas (self-hosted instance)
	if !PaymentsEnabled() {
		return true, false, 0, nil
	}

	cost := GetOperationCost(operation)

	// Check if user has sufficient credits
	balance := GetBalance(userID)
	if balance >= cost {
		return true, false, cost, nil
	}

	// User needs to top up
	return false, false, cost, errors.New("insufficient credits")
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

// ConsumeQuota consumes quota for an operation (call after successful operation)
func ConsumeQuota(userID string, operation string) error {
	// Get account to check admin status
	acc, err := auth.GetAccount(userID)
	if err != nil {
		return errors.New("account not found")
	}

	// Admins get unlimited access but usage is tracked
	if acc.Admin {
		RecordUsage(userID, operation)
		return nil
	}

	// Deduct credits
	cost := GetOperationCost(operation)
	return DeductCredits(userID, cost, operation, nil)
}

// HoldEscrow deducts credits from a user's wallet into escrow (held for a task)
func HoldEscrow(userID string, amount int, taskID string) error {
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

	tx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      TxSpend,
		Amount:    -amount,
		Balance:   w.Balance,
		Operation: OpEscrowHold,
		Metadata:  map[string]interface{}{"task_id": taskID},
		CreatedAt: time.Now(),
	}
	transactions[userID] = append(transactions[userID], tx)

	data.SaveJSON("wallets.json", wallets)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// ReleaseEscrow pays the worker from escrowed credits
func ReleaseEscrow(workerID string, amount int, taskID string) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	mutex.Lock()
	defer mutex.Unlock()

	w, exists := wallets[workerID]
	if !exists {
		w = &Wallet{
			UserID:   workerID,
			Balance:  0,
			Currency: "GBP",
		}
		wallets[workerID] = w
	}

	w.Balance += amount
	w.UpdatedAt = time.Now()

	tx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    workerID,
		Type:      TxTopup,
		Amount:    amount,
		Balance:   w.Balance,
		Operation: OpEscrowRelease,
		Metadata:  map[string]interface{}{"task_id": taskID},
		CreatedAt: time.Now(),
	}
	transactions[workerID] = append(transactions[workerID], tx)

	data.SaveJSON("wallets.json", wallets)
	data.SaveJSON("transactions.json", transactions)

	return nil
}

// RefundEscrow returns escrowed credits to the poster
func RefundEscrow(userID string, amount int, taskID string) error {
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

	tx := &Transaction{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      TxRefund,
		Amount:    amount,
		Balance:   w.Balance,
		Operation: OpEscrowRefund,
		Metadata:  map[string]interface{}{"task_id": taskID},
		CreatedAt: time.Now(),
	}
	transactions[userID] = append(transactions[userID], tx)

	data.SaveJSON("wallets.json", wallets)
	data.SaveJSON("transactions.json", transactions)

	return nil
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
		Operation: OpAppUse,
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
		Operation: OpAppRevenue,
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
