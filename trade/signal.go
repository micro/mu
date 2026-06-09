package trade

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/markets"
	"mu/news"
)

const evalInterval = 15 * time.Minute

// NotifyFunc is called when a signal is generated (alert/confirm mode).
// Set by the discord package to send DM notifications.
var NotifyFunc func(accountID, message string)

func StartSignalLoop() {
	go func() {
		for {
			time.Sleep(evalInterval)
			evaluateStrategies()
		}
	}()
	app.Log("trade", "Signal evaluation loop started (every %v)", evalInterval)
}

func evaluateStrategies() {
	active := GetActiveStrategies()
	if len(active) == 0 {
		return
	}

	ctx := buildMarketContext()
	if ctx == "" {
		return
	}

	for _, s := range active {
		evaluateStrategy(s, ctx)
	}
}

func buildMarketContext() string {
	var parts []string

	// Market prices
	priceData := markets.GetAllPriceData()
	if len(priceData) > 0 {
		var priceLine []string
		for symbol, pd := range priceData {
			changeStr := fmt.Sprintf("%.1f%%", pd.Change24h)
			if pd.Change24h > 0 {
				changeStr = "+" + changeStr
			}
			priceLine = append(priceLine, fmt.Sprintf("%s: $%.2f (%s 24h)", symbol, pd.Price, changeStr))
		}
		parts = append(parts, "## Current Prices\n"+strings.Join(priceLine, "\n"))
	}

	// Recent news headlines
	feed := news.GetFeed()
	if len(feed) > 10 {
		feed = feed[:10]
	}
	if len(feed) > 0 {
		var headlines []string
		for _, p := range feed {
			headlines = append(headlines, "- "+p.Title)
		}
		parts = append(parts, "## Recent News\n"+strings.Join(headlines, "\n"))
	}

	return strings.Join(parts, "\n\n")
}

func evaluateStrategy(s *Strategy, marketCtx string) {
	if !withinWeeklyLimit(s) {
		return
	}

	prompt := fmt.Sprintf(`You are a trading signal analyst. A user has the following trading strategy:

"%s"

Current market data and news:
%s

Based on the strategy and current conditions, should a trade be executed right now?

Respond with ONLY valid JSON in this exact format:
{"action":"none"} — if conditions are not met
{"action":"buy","token":"ETH","amount":"%s","reason":"brief explanation"} — if conditions are met for a buy
{"action":"sell","token":"ETH","amount":"%s","reason":"brief explanation"} — if conditions are met for a sell

Rules:
- Only trigger if the strategy conditions are clearly met
- Use the max_per_trade amount as the trade size
- Be conservative — don't trigger on weak signals
- The reason should cite specific data (price, news headline, percentage change)`, s.Description, marketCtx, s.MaxPerTrade, s.MaxPerTrade)

	result, err := ai.Ask(&ai.Prompt{
		System:   "You are a trading signal evaluator. Respond with ONLY JSON, no other text.",
		Question: prompt,
		Priority: ai.PriorityLow,
		Caller:   "trade-signal",
	})
	if err != nil {
		app.Log("trade", "Signal eval error for %s: %v", s.ID, err)
		return
	}

	var signal struct {
		Action string `json:"action"`
		Token  string `json:"token"`
		Amount string `json:"amount"`
		Reason string `json:"reason"`
	}

	result = extractJSON(result)
	if err := json.Unmarshal([]byte(result), &signal); err != nil {
		app.Log("trade", "Signal parse error for %s: %v (raw: %.200s)", s.ID, err, result)
		return
	}

	// Update last check
	strategyMu.Lock()
	if st, ok := strategies[s.ID]; ok {
		st.LastCheck = time.Now().UTC()
		st.LastSignal = signal.Action
		saveStrategies()
	}
	strategyMu.Unlock()

	if signal.Action == "none" || signal.Action == "" {
		return
	}

	app.Log("trade", "Signal triggered for %s: %s %s %s — %s", s.ID, signal.Action, signal.Amount, signal.Token, signal.Reason)

	sig := &Signal{
		StrategyID: s.ID,
		Account:    s.Account,
		Action:     signal.Action,
		Token:      signal.Token,
		Amount:     signal.Amount,
		Reason:     signal.Reason,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	switch s.Mode {
	case ModeAuto:
		executeSignal(s, sig)
	case ModeConfirm, ModeAlert:
		RecordSignal(sig)
		if NotifyFunc != nil {
			emoji := "📈"
			if sig.Action == "sell" {
				emoji = "📉"
			}
			msg := fmt.Sprintf("%s **Signal: %s %s %s**\n%s", emoji, sig.Action, sig.Amount, sig.Token, sig.Reason)
			NotifyFunc(s.Account, msg)
		}
	}
}

func executeSignal(s *Strategy, sig *Signal) {
	var fromToken, toToken string
	if sig.Action == "buy" {
		fromToken = "USDC"
		toToken = sig.Token
	} else {
		fromToken = sig.Token
		toToken = "USDC"
	}

	t, err := ExecuteSwap(s.Account, fromToken, toToken, sig.Amount)
	if err != nil {
		app.Log("trade", "Auto-execute failed for %s: %v", s.ID, err)
		sig.Reason += " (execution failed: " + err.Error() + ")"
		RecordSignal(sig)
		return
	}

	sig.Executed = true
	sig.TradeID = t.ID
	RecordSignal(sig)
	updateStrategySpend(s.ID, sig.Amount)

	app.Log("trade", "Auto-executed %s %s %s for strategy %s (tx: %s)", sig.Action, sig.Amount, sig.Token, s.ID, t.TxHash)

	if NotifyFunc != nil {
		msg := fmt.Sprintf("✅ **Executed: %s %s %s**\n%s\nTx: %s", sig.Action, sig.Amount, sig.Token, sig.Reason, t.TxHash)
		NotifyFunc(s.Account, msg)
	}
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return "{}"
	}
	return s[start : end+1]
}
