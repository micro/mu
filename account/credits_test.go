package account

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
		{quota.OpNewsSearch, quota.OperationCost(quota.OpNewsSearch)},
		{quota.OpVideoSearch, quota.OperationCost(quota.OpVideoSearch)},
		{quota.OpBlogCreate, quota.OperationCost(quota.OpBlogCreate)},
		{quota.OpMailSend, quota.OperationCost(quota.OpMailSend)},
		{quota.OpExternalEmail, quota.OperationCost(quota.OpExternalEmail)},
		{quota.OpPlacesSearch, quota.OperationCost(quota.OpPlacesSearch)},
		{quota.OpPlacesNearby, quota.OperationCost(quota.OpPlacesNearby)},
		{quota.OpWeatherForecast, quota.OperationCost(quota.OpWeatherForecast)},
		{quota.OpWeatherPollen, quota.OperationCost(quota.OpWeatherPollen)},
		{quota.OpWebFetch, quota.OperationCost(quota.OpWebFetch)},
		{quota.OpWebSearch, quota.OperationCost(quota.OpWebSearch)},
		{"unknown_op", 1}, // default
	}
	for _, tt := range tests {
		got := quota.OperationCost(tt.op)
		if got != tt.expected {
			t.Errorf("OperationCost(%q) = %d, want %d", tt.op, got, tt.expected)
		}
	}
}

func TestOperationConstants(t *testing.T) {
	// Ensure all operation constants are unique
	ops := []string{
		quota.OpNewsSearch, quota.OpVideoSearch, quota.OpBlogCreate,
		quota.OpMailSend, quota.OpExternalEmail, quota.OpPlacesSearch,
		quota.OpPlacesNearby, quota.OpWeatherForecast, quota.OpWeatherPollen,
		quota.OpWebSearch, quota.OpWebFetch,
		quota.OpTopup, quota.OpRefund,
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
	// A model call a caller *chose to make as a tool* costs something.
	//
	// This used to include chat and agent, on the flat rule that a model call is
	// never free. The rule was about arithmetic and the answer is a product
	// decision: talking to your agent is the thing you came for, and metering it
	// meant a new account could not ask a question until it had paid — which is
	// why a daily grant of credits existed to pay the charge straight back. The
	// agent is free and bounded by a count instead; a tool it calls still pays
	// for itself.
	for name, cost := range map[string]int{
		"image":     quota.OperationCost(quota.OpImageGenerate),
		"app build": quota.OperationCost(quota.OpAppBuild),
		"app edit":  quota.OperationCost(quota.OpAppEdit),
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
		"news search":  quota.OperationCost(quota.OpNewsSearch),
		"web fetch":    quota.OperationCost(quota.OpWebFetch),
		"video search": quota.OperationCost(quota.OpVideoSearch),
		"quran search": quota.OperationCost(quota.OpQuranSearch),
	} {
		if cost != 0 {
			t.Errorf("%s costs us nothing to run, so charging for it prices something that is not a cost", name)
		}
	}

	// An agent run is more model than a chat turn — a plan and a synthesis
	// against one call — so it cannot be priced below it. There was a premium
	// tier above both, on Anthropic at twenty credits; nothing sent the field
	// that selected it, so it went with the machinery.
	// An app build is one generation. It was 100 — six times the next most
	// expensive thing on the menu, and more than an agent run that may make
	// several model calls.
	// The agent is free now, so there is no agent price to be in proportion to.
	// Image generation is the yardstick instead: both are one expensive model
	// call, and a build that costs several times an image is out of step.
	if quota.OperationCost(quota.OpAppBuild) > 2*quota.OperationCost(quota.OpImageGenerate) {
		t.Errorf("app build at %d is out of proportion to an image at %d",
			quota.OperationCost(quota.OpAppBuild), quota.OperationCost(quota.OpImageGenerate))
	}
	if quota.OperationCost(quota.OpExternalEmail) <= quota.OperationCost(quota.OpMailSend) {
		t.Error("external email should cost more than internal mail")
	}
}

func TestGetWallet_CreatesNew(t *testing.T) {
	// Reset balances for test
	mutex.Lock()
	origWallets := balances
	balances = map[string]*Credits{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		mutex.Unlock()
	}()

	w := CreditsOf("test-user-new")
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
	origWallets := balances
	balances = map[string]*Credits{
		"cached-user": {UserID: "cached-user", Balance: 500, Currency: "GBP"},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		mutex.Unlock()
	}()

	w := CreditsOf("cached-user")
	if w.Balance != 500 {
		t.Errorf("expected balance 500, got %d", w.Balance)
	}
}

func TestGetBalance(t *testing.T) {
	mutex.Lock()
	origWallets := balances
	balances = map[string]*Credits{
		"balance-user": {UserID: "balance-user", Balance: 1000, Currency: "GBP"},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		mutex.Unlock()
	}()

	if Balance("balance-user") != 1000 {
		t.Errorf("expected 1000, got %d", Balance("balance-user"))
	}
}

func TestAddCredits(t *testing.T) {
	mutex.Lock()
	origWallets := balances
	origTx := transactions
	balances = map[string]*Credits{
		"add-user": {UserID: "add-user", Balance: 100, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		transactions = origTx
		mutex.Unlock()
	}()

	err := AddCredits("add-user", 500, quota.OpTopup, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Balance("add-user") != 600 {
		t.Errorf("expected balance 600, got %d", Balance("add-user"))
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
	origWallets := balances
	origTx := transactions
	balances = map[string]*Credits{
		"deduct-user": {UserID: "deduct-user", Balance: 100, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		transactions = origTx
		mutex.Unlock()
	}()

	err := DeductCredits("deduct-user", 30, quota.OpImageGenerate, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Balance("deduct-user") != 70 {
		t.Errorf("expected balance 70, got %d", Balance("deduct-user"))
	}
}

func TestDeductCredits_InsufficientBalance(t *testing.T) {
	mutex.Lock()
	origWallets := balances
	balances = map[string]*Credits{
		"poor-user": {UserID: "poor-user", Balance: 5, Currency: "GBP"},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		mutex.Unlock()
	}()

	err := DeductCredits("poor-user", 10, quota.OpImageGenerate, nil)
	if err == nil {
		t.Error("expected error for insufficient balance")
	}
}

func TestDeductCredits_NonexistentUser(t *testing.T) {
	mutex.Lock()
	origWallets := balances
	balances = map[string]*Credits{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		mutex.Unlock()
	}()

	err := DeductCredits("nobody", 10, quota.OpImageGenerate, nil)
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestTransferCreditsDailyCap(t *testing.T) {
	mutex.Lock()
	origWallets := balances
	origTx := transactions
	balances = map[string]*Credits{
		"sender":   {UserID: "sender", Balance: DailyTransferCap + 1000, Currency: "GBP"},
		"receiver": {UserID: "receiver", Balance: 0, Currency: "GBP"},
	}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		balances = origWallets
		transactions = origTx
		mutex.Unlock()
	}()

	if err := TransferCredits("sender", "receiver", DailyTransferCap); err != nil {
		t.Fatalf("first transfer unexpected error: %v", err)
	}
	if err := TransferCredits("sender", "receiver", 1); err == nil {
		t.Fatal("expected daily transfer cap error")
	}
	if got := Balance("sender"); got != 1000 {
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
			{Type: TxSpend, Operation: quota.OpWebSearch, Amount: -3, CreatedAt: now},
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
			{ID: "2", Amount: -5, Operation: quota.OpImageGenerate},
			{ID: "3", Amount: -3, Operation: quota.OpNewsSearch},
		},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		transactions = origTx
		mutex.Unlock()
	}()

	txs := Transactions("tx-user", 2)
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

	txs := Transactions("nobody", 10)
	if txs == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(txs) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(txs))
	}
}
