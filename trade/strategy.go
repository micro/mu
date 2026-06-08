package trade

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"mu/internal/data"
)

type ExecutionMode string

const (
	ModeAlert   ExecutionMode = "alert"
	ModeConfirm ExecutionMode = "confirm"
	ModeAuto    ExecutionMode = "auto"
)

type Strategy struct {
	ID          string        `json:"id"`
	Account     string        `json:"account"`
	Description string        `json:"description"`
	Mode        ExecutionMode `json:"mode"`
	MaxPerTrade string        `json:"max_per_trade"` // e.g. "50" (in USDC)
	MaxPerWeek  string        `json:"max_per_week"`  // e.g. "200" (in USDC)
	SpentWeek   string        `json:"spent_week"`    // spent this week
	WeekReset   time.Time     `json:"week_reset"`
	Active      bool          `json:"active"`
	CreatedAt   time.Time     `json:"created_at"`
	LastCheck   time.Time     `json:"last_check"`
	LastSignal  string        `json:"last_signal,omitempty"`
}

type Signal struct {
	StrategyID string `json:"strategy_id"`
	Account    string `json:"account"`
	Action     string `json:"action"`     // "buy" or "sell"
	Token      string `json:"token"`      // e.g. "ETH"
	Amount     string `json:"amount"`     // e.g. "50" (USDC value)
	Reason     string `json:"reason"`     // AI-generated explanation
	Executed   bool   `json:"executed"`
	TradeID    string `json:"trade_id,omitempty"`
	CreatedAt  string `json:"created_at"`
}

var (
	strategyMu sync.RWMutex
	strategies = map[string]*Strategy{}       // strategyID → strategy
	signals    = map[string][]*Signal{}        // accountID → signals
)

func loadStrategies() {
	data.LoadJSON("trade_strategies.json", &strategies)
	data.LoadJSON("trade_signals.json", &signals)
}

func saveStrategies() {
	data.SaveJSON("trade_strategies.json", strategies)
}

func saveSignals() {
	data.SaveJSON("trade_signals.json", signals)
}

func CreateStrategy(accountID, description string, mode ExecutionMode, maxPerTrade, maxPerWeek string) (*Strategy, error) {
	if description == "" {
		return nil, fmt.Errorf("strategy description required")
	}
	if mode == "" {
		mode = ModeAlert
	}
	if mode == ModeAuto && maxPerTrade == "" {
		return nil, fmt.Errorf("auto-execute requires max_per_trade limit")
	}
	if maxPerTrade == "" {
		maxPerTrade = "50"
	}
	if maxPerWeek == "" {
		maxPerWeek = "500"
	}

	w := GetWallet(accountID)
	if w == nil {
		return nil, fmt.Errorf("create a trading wallet first at /trade")
	}

	s := &Strategy{
		ID:          fmt.Sprintf("s_%d", time.Now().UnixNano()),
		Account:     accountID,
		Description: description,
		Mode:        mode,
		MaxPerTrade: maxPerTrade,
		MaxPerWeek:  maxPerWeek,
		SpentWeek:   "0",
		WeekReset:   nextMonday(),
		Active:      true,
		CreatedAt:   time.Now().UTC(),
	}

	strategyMu.Lock()
	strategies[s.ID] = s
	saveStrategies()
	strategyMu.Unlock()

	return s, nil
}

func GetStrategies(accountID string) []*Strategy {
	strategyMu.RLock()
	defer strategyMu.RUnlock()

	var result []*Strategy
	for _, s := range strategies {
		if s.Account == accountID {
			result = append(result, s)
		}
	}
	return result
}

func GetActiveStrategies() []*Strategy {
	strategyMu.RLock()
	defer strategyMu.RUnlock()

	var result []*Strategy
	for _, s := range strategies {
		if s.Active {
			result = append(result, s)
		}
	}
	return result
}

func PauseStrategy(accountID, strategyID string) error {
	strategyMu.Lock()
	defer strategyMu.Unlock()

	s, ok := strategies[strategyID]
	if !ok || s.Account != accountID {
		return fmt.Errorf("strategy not found")
	}
	s.Active = !s.Active
	saveStrategies()
	return nil
}

func DeleteStrategy(accountID, strategyID string) error {
	strategyMu.Lock()
	defer strategyMu.Unlock()

	s, ok := strategies[strategyID]
	if !ok || s.Account != accountID {
		return fmt.Errorf("strategy not found")
	}
	delete(strategies, strategyID)
	saveStrategies()
	return nil
}

func RecordSignal(sig *Signal) {
	strategyMu.Lock()
	defer strategyMu.Unlock()

	signals[sig.Account] = append(signals[sig.Account], sig)
	if len(signals[sig.Account]) > 100 {
		signals[sig.Account] = signals[sig.Account][len(signals[sig.Account])-100:]
	}
	saveSignals()
}

func GetSignals(accountID string, limit int) []*Signal {
	strategyMu.RLock()
	defer strategyMu.RUnlock()

	sigs := signals[accountID]
	if len(sigs) <= limit {
		return sigs
	}
	return sigs[len(sigs)-limit:]
}

func updateStrategySpend(strategyID, amount string) {
	strategyMu.Lock()
	defer strategyMu.Unlock()

	s, ok := strategies[strategyID]
	if !ok {
		return
	}

	// Reset weekly spend if past the reset time
	if time.Now().After(s.WeekReset) {
		s.SpentWeek = "0"
		s.WeekReset = nextMonday()
	}

	spent, _ := ParseAmount(s.SpentWeek, 6)
	add, _ := ParseAmount(amount, 6)
	if spent != nil && add != nil {
		spent.Add(spent, add)
		s.SpentWeek = FormatAmount(spent, 6)
	}
	saveStrategies()
}

func withinWeeklyLimit(s *Strategy) bool {
	if time.Now().After(s.WeekReset) {
		return true
	}
	spent, _ := ParseAmount(s.SpentWeek, 6)
	limit, _ := ParseAmount(s.MaxPerWeek, 6)
	if spent == nil || limit == nil {
		return true
	}
	return spent.Cmp(limit) < 0
}

func nextMonday() time.Time {
	now := time.Now().UTC()
	daysUntilMonday := (8 - int(now.Weekday())) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	return time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 0, 0, 0, 0, time.UTC)
}

// DeleteStrategies removes all strategies for a user (account deletion).
func DeleteStrategies(accountID string) {
	strategyMu.Lock()
	defer strategyMu.Unlock()

	for id, s := range strategies {
		if s.Account == accountID {
			delete(strategies, id)
		}
	}
	delete(signals, accountID)
	saveStrategies()
	saveSignals()
}

var _ = json.Marshal

type StrategyPreset struct {
	Name        string
	Description string
}

var strategyPresets = []StrategyPreset{
	{
		Name:        "Buy the dip",
		Description: "Buy ETH when the price drops more than 5% in 24 hours and recent news sentiment is not negative",
	},
	{
		Name:        "News-driven ETH",
		Description: "Buy ETH when there are positive Ethereum news headlines (upgrades, ETF, adoption) and the price hasn't already pumped more than 3% today",
	},
	{
		Name:        "BTC momentum",
		Description: "Buy BTC when the price is up more than 2% in 24 hours and there are bullish crypto news headlines suggesting continued momentum",
	},
	{
		Name:        "Sell the rally",
		Description: "Sell ETH for USDC when the price rises more than 8% in 24 hours — take profits on sharp moves up",
	},
	{
		Name:        "Stablecoin on fear",
		Description: "Sell ETH for USDC when there are multiple negative crypto news headlines (regulation, hacks, crashes) and the price is already dropping",
	},
}
