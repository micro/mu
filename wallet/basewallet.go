package wallet

// Per-user Base (Ethereum L2) wallet: every account gets a secp256k1 keypair
// used to pay for MCP/x402 calls. This owns key storage, balance reads, and the
// minimal JSON-RPC needed — no external chain dependency. Ported and slimmed
// from the retired trade package (keys persist in the same file so existing
// wallets carry over).

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
	"mu/internal/settings"
)

// USDC on Base mainnet (6 decimals) — the asset x402 settles in.
const (
	baseUSDC        = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	baseUSDCDecimal = 6
)

// BaseWallet is a user's on-chain wallet.
type BaseWallet struct {
	Address    string `json:"address"`
	PrivateKey string `json:"private_key"` // hex, 32 bytes
}

var (
	walletMu    sync.RWMutex
	userWallets = map[string]*BaseWallet{} // accountID → wallet
	walletsFile = "trade_wallets.json"     // kept for continuity with existing keys
	walletsInit sync.Once
)

func loadWallets() {
	walletsInit.Do(func() {
		walletMu.Lock()
		defer walletMu.Unlock()
		data.LoadJSON(walletsFile, &userWallets)
	})
}

// BaseRPCURL returns the Base JSON-RPC endpoint. Honours BASE_RPC_URL, then the
// legacy TRADE_RPC_URL, then a public default.
func BaseRPCURL() string {
	if v := settings.Get("BASE_RPC_URL"); v != "" {
		return v
	}
	if v := settings.Get("TRADE_RPC_URL"); v != "" {
		return v
	}
	return "https://mainnet.base.org"
}

// WalletFor returns the account's wallet, or nil if it has none yet.
func WalletFor(accountID string) *BaseWallet {
	loadWallets()
	walletMu.RLock()
	defer walletMu.RUnlock()
	return userWallets[accountID]
}

// GetOrCreateWallet returns the account's wallet, generating one on first use.
func GetOrCreateWallet(accountID string) (*BaseWallet, error) {
	loadWallets()
	walletMu.Lock()
	defer walletMu.Unlock()
	if w, ok := userWallets[accountID]; ok {
		return w, nil
	}
	priv, addr, err := GenerateKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}
	w := &BaseWallet{Address: addr, PrivateKey: priv}
	userWallets[accountID] = w
	data.SaveJSON(walletsFile, userWallets)
	return w, nil
}

// DeleteBaseWallet removes an account's on-chain wallet (account teardown).
func DeleteBaseWallet(accountID string) {
	loadWallets()
	walletMu.Lock()
	defer walletMu.Unlock()
	if _, ok := userWallets[accountID]; ok {
		delete(userWallets, accountID)
		data.SaveJSON(walletsFile, userWallets)
	}
}

// USDCBalance returns the wallet's USDC balance as a formatted decimal string
// (e.g. "1.50") and the raw atomic amount.
func USDCBalance(address string) (string, *big.Int) {
	raw, err := tokenBalance(baseUSDC, address)
	if err != nil || raw == nil {
		return "0", big.NewInt(0)
	}
	return FormatUnits(raw, baseUSDCDecimal), raw
}

// ── minimal JSON-RPC ────────────────────────────────────────────────────────

var rpcClient = &http.Client{Timeout: 15 * time.Second}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func rpcCall(method string, params ...any) (json.RawMessage, error) {
	url := BaseRPCURL()
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := rpcClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out rpcResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("rpc parse error: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", out.Error.Message)
	}
	return out.Result, nil
}

// tokenBalance calls ERC-20 balanceOf(address).
func tokenBalance(token, wallet string) (*big.Int, error) {
	addr := strings.TrimPrefix(strings.ToLower(wallet), "0x")
	callData := "0x70a08231" + fmt.Sprintf("%064s", addr) // balanceOf selector + padded addr
	res, err := rpcCall("eth_call", map[string]string{"to": token, "data": callData}, "latest")
	if err != nil {
		return nil, err
	}
	return hexToBigInt(strings.Trim(string(res), `"`)), nil
}

func hexToBigInt(s string) *big.Int {
	s = strings.TrimPrefix(s, "0x")
	if v, ok := new(big.Int).SetString(s, 16); ok {
		return v
	}
	return big.NewInt(0)
}

// FormatUnits renders a raw integer amount with the given decimals, trimming
// trailing zeros (e.g. 1500000 @ 6 → "1.5").
func FormatUnits(raw *big.Int, decimals int) string {
	if raw == nil || raw.Sign() == 0 {
		return "0"
	}
	s := raw.String()
	if decimals == 0 {
		return s
	}
	for len(s) <= decimals {
		s = "0" + s
	}
	intPart, fracPart := s[:len(s)-decimals], s[len(s)-decimals:]
	fracPart = strings.TrimRight(fracPart, "0")
	if fracPart == "" {
		return intPart
	}
	return intPart + "." + fracPart
}

var _ = hex.EncodeToString
