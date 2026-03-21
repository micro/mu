package wallet

import (
	"testing"
)

func TestFormatCredits(t *testing.T) {
	tests := []struct {
		credits  int
		expected string
	}{
		{0, "£0.00"},
		{1, "£0.01"},
		{50, "£0.50"},
		{100, "£1.00"},
		{1550, "£15.50"},
		{10000, "£100.00"},
	}
	for _, tt := range tests {
		got := FormatCredits(tt.credits)
		if got != tt.expected {
			t.Errorf("FormatCredits(%d) = %q, want %q", tt.credits, got, tt.expected)
		}
	}
}

func TestGetOperationCost(t *testing.T) {
	tests := []struct {
		op       string
		expected int
	}{
		{OpNewsSearch, CostNewsSearch},
		{OpVideoSearch, CostVideoSearch},
		{OpChatQuery, CostChatQuery},
		{OpBlogCreate, CostBlogCreate},
		{OpMailSend, CostMailSend},
		{OpExternalEmail, CostExternalEmail},
		{OpPlacesSearch, CostPlacesSearch},
		{OpPlacesNearby, CostPlacesNearby},
		{OpWeatherForecast, CostWeatherForecast},
		{OpWeatherPollen, CostWeatherPollen},
		{OpWebSearch, CostWebSearch},
		{OpWebFetch, CostWebFetch},
		{OpAgentQuery, CostAgentQuery},
		{OpAgentQueryPremium, CostAgentQueryPremium},
		{"unknown_op", 1}, // default
	}
	for _, tt := range tests {
		got := GetOperationCost(tt.op)
		if got != tt.expected {
			t.Errorf("GetOperationCost(%q) = %d, want %d", tt.op, got, tt.expected)
		}
	}
}

func TestOperationConstants(t *testing.T) {
	// Ensure all operation constants are unique
	ops := []string{
		OpNewsSearch, OpVideoSearch, OpChatQuery, OpBlogCreate,
		OpMailSend, OpExternalEmail, OpPlacesSearch,
		OpPlacesNearby, OpWeatherForecast, OpWeatherPollen,
		OpWebSearch, OpWebFetch, OpAgentQuery,
		OpAgentQueryPremium, OpTopup, OpRefund,
	}
	seen := make(map[string]bool)
	for _, op := range ops {
		if seen[op] {
			t.Errorf("duplicate operation constant: %q", op)
		}
		seen[op] = true
	}
}

func TestTransactionTypeConstants(t *testing.T) {
	if TxTopup != "topup" {
		t.Errorf("unexpected TxTopup: %q", TxTopup)
	}
	if TxSpend != "spend" {
		t.Errorf("unexpected TxSpend: %q", TxSpend)
	}
	if TxRefund != "refund" {
		t.Errorf("unexpected TxRefund: %q", TxRefund)
	}
}

func TestDefaultCosts(t *testing.T) {
	// Verify default cost values are reasonable
	if CostNewsSearch < 1 {
		t.Error("news search cost should be >= 1")
	}
	if CostChatQuery < 1 {
		t.Error("chat query cost should be >= 1")
	}
	if CostAgentQueryPremium <= CostAgentQuery {
		t.Error("premium agent should cost more than standard")
	}
	if CostExternalEmail <= CostMailSend {
		t.Error("external email should cost more than internal mail")
	}
	if FreeDailyQuota < 1 {
		t.Error("free daily quota should be >= 1")
	}
}

func TestGetWallet_CreatesNew(t *testing.T) {
	// Reset wallets for test
	mutex.Lock()
	origWallets := wallets
	wallets = map[string]*Wallet{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		mutex.Unlock()
	}()

	w := GetWallet("test-user-new")
	if w == nil {
		t.Fatal("expected wallet to be created")
	}
	if w.UserID != "test-user-new" {
		t.Errorf("expected user_id 'test-user-new', got %q", w.UserID)
	}
	if w.Balance != 0 {
		t.Errorf("expected 0 balance, got %d", w.Balance)
	}
	if w.Currency != "GBP" {
		t.Errorf("expected GBP currency, got %q", w.Currency)
	}
}

func TestGetWallet_ReturnsCached(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	wallets = map[string]*Wallet{
		"cached-user": {UserID: "cached-user", Balance: 500, Currency: "GBP"},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		mutex.Unlock()
	}()

	w := GetWallet("cached-user")
	if w.Balance != 500 {
		t.Errorf("expected balance 500, got %d", w.Balance)
	}
}

func TestGetBalance(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	wallets = map[string]*Wallet{
		"balance-user": {UserID: "balance-user", Balance: 1000, Currency: "GBP"},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		mutex.Unlock()
	}()

	if GetBalance("balance-user") != 1000 {
		t.Errorf("expected 1000, got %d", GetBalance("balance-user"))
	}
}

func TestAddCredits(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	origTx := transactions
	wallets = map[string]*Wallet{
		"add-user": {UserID: "add-user", Balance: 100, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		transactions = origTx
		mutex.Unlock()
	}()

	err := AddCredits("add-user", 500, OpTopup, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if GetBalance("add-user") != 600 {
		t.Errorf("expected balance 600, got %d", GetBalance("add-user"))
	}
}

func TestAddCredits_NegativeAmount(t *testing.T) {
	err := AddCredits("user", -10, OpTopup, nil)
	if err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestAddCredits_ZeroAmount(t *testing.T) {
	err := AddCredits("user", 0, OpTopup, nil)
	if err == nil {
		t.Error("expected error for zero amount")
	}
}

func TestDeductCredits(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	origTx := transactions
	wallets = map[string]*Wallet{
		"deduct-user": {UserID: "deduct-user", Balance: 100, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		transactions = origTx
		mutex.Unlock()
	}()

	err := DeductCredits("deduct-user", 30, OpChatQuery, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if GetBalance("deduct-user") != 70 {
		t.Errorf("expected balance 70, got %d", GetBalance("deduct-user"))
	}
}

func TestDeductCredits_InsufficientBalance(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	wallets = map[string]*Wallet{
		"poor-user": {UserID: "poor-user", Balance: 5, Currency: "GBP"},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		mutex.Unlock()
	}()

	err := DeductCredits("poor-user", 10, OpChatQuery, nil)
	if err == nil {
		t.Error("expected error for insufficient balance")
	}
}

func TestDeductCredits_NonexistentUser(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	wallets = map[string]*Wallet{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		mutex.Unlock()
	}()

	err := DeductCredits("nobody", 10, OpChatQuery, nil)
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestGetTransactions(t *testing.T) {
	mutex.Lock()
	origTx := transactions
	transactions = map[string][]*Transaction{
		"tx-user": {
			{ID: "1", Amount: 100, Operation: OpTopup},
			{ID: "2", Amount: -5, Operation: OpChatQuery},
			{ID: "3", Amount: -3, Operation: OpNewsSearch},
		},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		transactions = origTx
		mutex.Unlock()
	}()

	txs := GetTransactions("tx-user", 2)
	if len(txs) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txs))
	}
	// Should be newest first
	if txs[0].ID != "3" {
		t.Errorf("expected newest first (ID '3'), got %q", txs[0].ID)
	}
}

func TestGetTransactions_EmptyUser(t *testing.T) {
	mutex.Lock()
	origTx := transactions
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		transactions = origTx
		mutex.Unlock()
	}()

	txs := GetTransactions("nobody", 10)
	if txs == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(txs) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(txs))
	}
}
