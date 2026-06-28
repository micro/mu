package trade

import (
	"context"
	"fmt"
)

// Server is the go-micro service handler for trading. Its methods are exposed
// as RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// QuoteRequest asks for a swap price quote.
type QuoteRequest struct {
	From   string `json:"from" description:"Token to sell (ETH, WETH, USDC)"`
	To     string `json:"to" description:"Token to buy (ETH, WETH, USDC)"`
	Amount string `json:"amount" description:"Amount to sell (e.g. 0.1 for 0.1 ETH)"`
}

// Quote returns a swap price quote for tokens on Base via Uniswap V3.
// @example {"from": "ETH", "to": "USDC", "amount": "0.1"}
func (Server) Quote(_ context.Context, req *QuoteRequest, rsp *Quote) error {
	q, err := GetQuote(req.From, req.To, req.Amount)
	if err != nil {
		return err
	}
	*rsp = *q
	return nil
}

// SwapRequest executes a token swap for an account.
type SwapRequest struct {
	AccountID string `json:"account_id" description:"Account performing the swap"`
	From      string `json:"from" description:"Token to sell"`
	To        string `json:"to" description:"Token to buy"`
	Amount    string `json:"amount" description:"Amount to sell"`
}

// Swap executes a token swap on Base via Uniswap V3.
func (Server) Swap(_ context.Context, req *SwapRequest, rsp *Trade) error {
	t, err := ExecuteSwap(req.AccountID, req.From, req.To, req.Amount)
	if err != nil {
		return err
	}
	*rsp = *t
	return nil
}

// WalletRequest asks for an account's trading wallet.
type WalletRequest struct {
	AccountID string `json:"account_id" description:"Account whose wallet to return"`
}

// Wallet returns the trading wallet address for an account.
func (Server) Wallet(_ context.Context, req *WalletRequest, rsp *WalletInfo) error {
	info := GetWalletInfo(req.AccountID)
	if info == nil {
		return fmt.Errorf("no trading wallet")
	}
	*rsp = *info
	return nil
}

// StrategyRequest creates an automated trading strategy.
type StrategyRequest struct {
	AccountID   string `json:"account_id" description:"Account that owns the strategy"`
	Description string `json:"description" description:"Strategy in plain English"`
	Mode        string `json:"mode" description:"alert, confirm or auto"`
	MaxPerTrade string `json:"max_per_trade" description:"Maximum USDC per trade"`
	MaxPerWeek  string `json:"max_per_week" description:"Maximum USDC per week"`
}

// Strategy creates an automated trading strategy that the system evaluates on a
// schedule and acts on when conditions are met.
func (Server) Strategy(_ context.Context, req *StrategyRequest, rsp *Strategy) error {
	mode := ExecutionMode(req.Mode)
	if mode == "" {
		mode = ModeAlert
	}
	s, err := CreateStrategy(req.AccountID, req.Description, mode, req.MaxPerTrade, req.MaxPerWeek)
	if err != nil {
		return err
	}
	*rsp = *s
	return nil
}
