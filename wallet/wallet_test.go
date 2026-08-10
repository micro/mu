package wallet

import (
	"testing"
	"time"

	"mu/internal/quota"
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
		{quota.OpNewsSearch, quota.CostNewsSearch},
		{quota.OpVideoSearch, quota.CostVideoSearch},
		{quota.OpChatQuery, quota.CostChatQuery},
		{quota.OpBlogCreate, quota.CostBlogCreate},
		{quota.OpMailSend, quota.CostMailSend},
		{quota.OpExternalEmail, quota.CostExternalEmail},
		{quota.OpPlacesSearch, quota.CostPlacesSearch},
		{quota.OpPlacesNearby, quota.CostPlacesNearby},
		{quota.OpWeatherForecast, quota.CostWeatherForecast},
		{quota.OpWeatherPollen, quota.CostWeatherPollen},
		{quota.OpWebSearch, quota.CostWebSearch},
		{quota.OpWebFetch, quota.CostWebFetch},
		{quota.OpAgentQuery, quota.CostAgentQuery},
		{quota.OpAgentQueryPremium, quota.CostAgentQueryPremium},
		{"unknown_op", 1}, // default
	}
	for _, tt := range tests {
		got := quota.GetOperationCost(tt.op)
		if got != tt.expected {
			t.Errorf("GetOperationCost(%q) = %d, want %d", tt.op, got, tt.expected)
		}
	}
}

func TestOperationConstants(t *testing.T) {
	// Ensure all operation constants are unique
	ops := []string{
		quota.OpNewsSearch, quota.OpVideoSearch, quota.OpChatQuery, quota.OpBlogCreate,
		quota.OpMailSend, quota.OpExternalEmail, quota.OpPlacesSearch,
		quota.OpPlacesNearby, quota.OpWeatherForecast, quota.OpWeatherPollen,
		quota.OpWebSearch, quota.OpWebFetch, quota.OpAgentQuery,
		quota.OpAgentQueryPremium, quota.OpTopup, quota.OpRefund,
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
	// A model call always costs something.
	for name, cost := range map[string]int{
		"chat":          quota.CostChatQuery,
		"agent":         quota.CostAgentQuery,
		"agent premium": quota.CostAgentQueryPremium,
		"image":         quota.CostImageGenerate,
		"app build":     quota.CostAppBuild,
		"app edit":      quota.CostAppEdit,
	} {
		if cost < 1 {
			t.Errorf("%s calls a model, so it cannot be free", name)
		}
	}

	// Nothing bills this instance for these, so they are not priced. Searching
	// news is a local index query; fetching a page is an http.Get and a
	// readability pass; searching video is a free (quota'd) API rationed by
	// service/video/searchlimit.go instead.
	for name, cost := range map[string]int{
		"news search":  quota.CostNewsSearch,
		"web fetch":    quota.CostWebFetch,
		"video search": quota.CostVideoSearch,
		"quran search": quota.CostQuranSearch,
	} {
		if cost != 0 {
			t.Errorf("%s costs us nothing to run, so charging for it prices something that is not a cost", name)
		}
	}

	// The premium tier routes to a materially more expensive provider, and was
	// priced 29% above standard for roughly 10-20x the cost.
	if quota.CostAgentQueryPremium < 2*quota.CostAgentQuery {
		t.Error("premium agent should cost enough more than standard to cover a different provider")
	}
	// An app build is one generation. It was 100 — six times the next most
	// expensive thing on the menu, and more than an agent run that may make
	// several model calls.
	if quota.CostAppBuild > 3*quota.CostAgentQuery {
		t.Errorf("app build at %d is out of proportion to an agent run at %d", quota.CostAppBuild, quota.CostAgentQuery)
	}
	if quota.CostExternalEmail <= quota.CostMailSend {
		t.Error("external email should cost more than internal mail")
	}
	if quota.DailyQuota < 1 {
		t.Error("daily quota should be >= 1")
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

	err := AddCredits("add-user", 500, quota.OpTopup, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if GetBalance("add-user") != 600 {
		t.Errorf("expected balance 600, got %d", GetBalance("add-user"))
	}
}

func TestAddCredits_NegativeAmount(t *testing.T) {
	err := AddCredits("user", -10, quota.OpTopup, nil)
	if err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestAddCredits_ZeroAmount(t *testing.T) {
	err := AddCredits("user", 0, quota.OpTopup, nil)
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

	err := DeductCredits("deduct-user", 30, quota.OpChatQuery, nil)
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

	err := DeductCredits("poor-user", 10, quota.OpChatQuery, nil)
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

	err := DeductCredits("nobody", 10, quota.OpChatQuery, nil)
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestTransferCreditsDailyCap(t *testing.T) {
	mutex.Lock()
	origWallets := wallets
	origTx := transactions
	wallets = map[string]*Wallet{
		"sender":   {UserID: "sender", Balance: DailyTransferCap + 1000, Currency: "GBP"},
		"receiver": {UserID: "receiver", Balance: 0, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		wallets = origWallets
		transactions = origTx
		mutex.Unlock()
	}()

	if err := TransferCredits("sender", "receiver", DailyTransferCap); err != nil {
		t.Fatalf("first transfer unexpected error: %v", err)
	}
	if err := TransferCredits("sender", "receiver", 1); err == nil {
		t.Fatal("expected daily transfer cap error")
	}
	if got := GetBalance("sender"); got != 1000 {
		t.Fatalf("sender balance after blocked transfer = %d, want 1000", got)
	}
}

func TestDailyTransferTotalIgnoresIncomingAndOldTransfers(t *testing.T) {
	mutex.Lock()
	origTx := transactions
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	transactions = map[string][]*Transaction{
		"user": {
			{Type: TxTransfer, Operation: quota.OpTransfer, Amount: -25, CreatedAt: now},
			{Type: TxTransfer, Operation: quota.OpTransfer, Amount: 15, CreatedAt: now},
			{Type: TxTransfer, Operation: quota.OpTransfer, Amount: -40, CreatedAt: now.AddDate(0, 0, -1)},
			{Type: TxSpend, Operation: quota.OpAgentQuery, Amount: -3, CreatedAt: now},
		},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		transactions = origTx
		mutex.Unlock()
	}()

	if got := DailyTransferTotal("user", now); got != 25 {
		t.Fatalf("daily transfer total = %d, want 25", got)
	}
}

func TestGetTransactions(t *testing.T) {
	mutex.Lock()
	origTx := transactions
	transactions = map[string][]*Transaction{
		"tx-user": {
			{ID: "1", Amount: 100, Operation: quota.OpTopup},
			{ID: "2", Amount: -5, Operation: quota.OpChatQuery},
			{ID: "3", Amount: -3, Operation: quota.OpNewsSearch},
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
