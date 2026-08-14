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

	"mu/internal/app"
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
	walletsFile = "wallets.json"
	walletsInit sync.Once
)

// legacyWalletsFile is where these keys lived when the only thing an account's
// wallet did was trade. It pays for tool calls now, and holds the balance a
// person topped up with, so the name had stopped describing the contents.
//
// It is read, never written, and never deleted. Anything else risks an
// instance coming up against the new name, finding nothing, and minting fresh
// wallets for accounts whose funds are sitting in keys we just stopped
// reading — a silent way to lose real money.
const legacyWalletsFile = "trade_wallets.json"

func loadWallets() {
	walletsInit.Do(func() {
		walletMu.Lock()
		defer walletMu.Unlock()
		userWallets = loadWalletsFrom(walletsFile, legacyWalletsFile)
	})
}

// loadWalletsFrom reads the wallet map, falling back to the pre-rename file and
// copying it forward. Separate from loadWallets so it can be tested without
// fighting the sync.Once that guards process-wide state.
func loadWalletsFrom(primary, legacy string) map[string]*BaseWallet {
	m := map[string]*BaseWallet{}
	_ = data.LoadJSON(primary, &m)
	if len(m) > 0 {
		return m
	}
	_ = data.LoadJSON(legacy, &m)
	if len(m) > 0 {
		_ = data.SaveJSON(primary, m)
	}
	return m
}

// BaseRPCURL returns the Base JSON-RPC endpoint. Honours BASE_RPC_URL, then the
// legacy TRADE_RPC_URL, then a public default.
func BaseRPCURL() string {
	if v := settings.Get("BASE_RPC_URL"); v != "" {
		return v
	}
	// TRADE_RPC_URL is a fallback and an assumption: it was set for trading and
	// nothing says it is on Base. An Alchemy key is per-chain — eth-mainnet and
	// base-mainnet are different hostnames — so a trading node configured for
	// Ethereum answers every Base token read with "0x". See checkChain, which
	// is what turns that into a sentence instead of a zero.
	if v := settings.Get("TRADE_RPC_URL"); v != "" {
		return v
	}
	return "https://mainnet.base.org"
}

// expectedChain is the chain this instance's money is on, from the configured
// x402 network.
func expectedChain() int64 {
	if id, ok := chainIDFor(x402Net()); ok {
		return id
	}
	return 8453
}

// chainOf asks a node which chain it is on, once per URL.
//
// Only ever called when something has already looked wrong, so the happy path
// pays nothing for it. Cached because the answer cannot change without the URL
// changing.
var chainSeen sync.Map // url -> int64

func chainOf(url string) (int64, error) {
	if v, ok := chainSeen.Load(url); ok {
		return v.(int64), nil
	}
	res, err := rpcCall("eth_chainId")
	if err != nil {
		return 0, err
	}
	hexed := strings.TrimPrefix(strings.Trim(string(res), `"`), "0x")
	id, ok := new(big.Int).SetString(hexed, 16)
	if !ok {
		return 0, fmt.Errorf("%s did not say which chain it is on", url)
	}
	chainSeen.Store(url, id.Int64())
	return id.Int64(), nil
}

// checkChain names the misconfiguration behind an empty answer.
//
// The failure this exists for: a token balance reads as zero while a block
// explorer shows real money at the same address. The node is fine, the address
// is fine, and the call is being made on the wrong chain — which no amount of
// staring at either will reveal.
func checkChain() error {
	url := BaseRPCURL()
	got, err := chainOf(url)
	if err != nil {
		return nil // unreachable is a different complaint, already reported
	}
	if want := expectedChain(); got != want {
		return fmt.Errorf("%s is on chain %d, but this instance's funds are on chain %d — "+
			"set BASE_RPC_URL to a node for chain %d (TRADE_RPC_URL is being used as a "+
			"fallback and is for a different chain)", url, got, want, want)
	}
	return nil
}

// WalletFor returns the account's wallet, or nil if it has none yet.
func WalletFor(accountID string) *BaseWallet {
	loadWallets()
	walletMu.RLock()
	defer walletMu.RUnlock()
	return userWallets[accountID]
}

// GetOrCreateWallet returns the account's wallet, generating one on first use.
//
// A stored record is checked rather than trusted. One with a key but no address
// used to come straight back with no error, and everything downstream then
// rendered an empty string where an address goes — the /wallet card drew a
// blank button and a QR code of nothing, and said nothing was wrong. A record
// can end up that way from a half-written file or a migration between two
// shapes of the same struct.
//
// The address is derived from the key rather than replaced. Minting a fresh
// wallet would be the easy repair and the wrong one: the old key may hold real
// USDC, and issuing a new address strands it silently.
func GetOrCreateWallet(accountID string) (*BaseWallet, error) {
	loadWallets()
	walletMu.Lock()
	defer walletMu.Unlock()
	if w, ok := userWallets[accountID]; ok && w != nil {
		if w.Address != "" {
			return w, nil
		}
		if addr, ok := AddressFromPrivateKeyHex(w.PrivateKey); ok {
			w.Address = addr
			data.SaveJSON(walletsFile, userWallets) //nolint:errcheck
			app.Log("wallet", "repaired the address on %s's wallet from its key", accountID)
			return w, nil
		}
		// A record with neither is not a wallet. Falling through mints one,
		// which is safe precisely because there is no key to strand.
		app.Log("wallet", "%s had a wallet record with no address and no usable key; minting", accountID)
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
	human, raw, _ := USDCBalanceErr(address)
	return human, raw
}

// USDCBalanceErr is the same read, saying whether it worked.
//
// The plain version reports an unreachable node as a zero balance, which is the
// wrong answer to the only question anybody asks this: somebody who has just
// sent money refreshes, sees nothing, and cannot tell whether the transfer has
// not landed or whether we failed to look. Those want different reactions —
// wait, or check the chain yourself — so a caller that can say which should.
func USDCBalanceErr(address string) (string, *big.Int, error) {
	raw, err := tokenBalance(baseUSDC, address)
	if err != nil {
		return "0", big.NewInt(0), err
	}
	if raw == nil {
		return "0", big.NewInt(0), fmt.Errorf("no balance returned for %s", address)
	}
	return FormatUnits(raw, baseUSDCDecimal), raw, nil
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
	if addr == "" {
		return nil, fmt.Errorf("no address to read a balance for")
	}
	callData := "0x70a08231" + fmt.Sprintf("%064s", addr) // balanceOf selector + padded addr
	res, err := rpcCall("eth_call", map[string]string{"to": token, "data": callData}, "latest")
	if err != nil {
		return nil, err
	}

	// An empty result is not a balance of zero.
	//
	// eth_call against an address that holds no contract returns "0x", and
	// hexToBigInt turns anything it cannot parse into zero — so a node pointed
	// at the wrong chain reports every token balance as nought, with no error,
	// forever. That is indistinguishable from an empty wallet and it is what
	// somebody staring at real money on a block explorer is looking at.
	//
	// BaseRPCURL falls back to TRADE_RPC_URL, which was set for trading and may
	// point anywhere, so this is a configuration away rather than a theory.
	hexed := strings.TrimPrefix(strings.Trim(string(res), `"`), "0x")
	if hexed == "" {
		// Ask the node what chain it is on, so the answer is the actual
		// misconfiguration rather than a guess at it.
		if err := checkChain(); err != nil {
			return nil, err
		}
		// The chain could not be established either, so name the two settings
		// that decide it — that is the actionable half whatever the cause.
		return nil, fmt.Errorf("%s returned no data for a balance on %s — that address "+
			"holds no token contract there, which usually means the node is on the wrong "+
			"chain (check BASE_RPC_URL, and TRADE_RPC_URL which is used as a fallback)",
			BaseRPCURL(), token)
	}
	v, ok := new(big.Int).SetString(hexed, 16)
	if !ok {
		return nil, fmt.Errorf("%s returned a balance that is not a number: %q", BaseRPCURL(), string(res))
	}
	return v, nil
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
