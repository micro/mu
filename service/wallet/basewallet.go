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

	"mu/internal/x402"
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
	// Named for what it holds. It used to be "wallets.json" — the same file the
	// credit ledger in wallet.go writes — and the two maps overwrote each other
	// on every save, in whichever order they happened to run.
	//
	// It destroyed both. Loading this file into BaseWallet structs gave every
	// account an entry with no address and no key, because the credit ledger's
	// fields have different names; saving that map back wiped every balance.
	// An account with 560 credits read zero, and a wallet holding real USDC
	// showed no address to send to.
	//
	// Two stores, two files. TestNoTwoStoresShareAFile holds the line.
	walletsFile = "base_wallets.json"
	walletsInit sync.Once
)

// legacyWalletsFile is where these keys lived when the only thing an account's
// wallet did was trade. It is also where they survived the collision above,
// because it is only ever read and the primary was never empty enough to reach
// it — which is the sole reason nobody's USDC was stranded. It pays for tool calls now, and holds the balance a
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
	m := usable(loadRaw(primary))
	if len(m) > 0 {
		return m
	}
	m = usable(loadRaw(legacy))
	if len(m) > 0 {
		_ = data.SaveJSON(primary, m)
	}
	return m
}

func loadRaw(file string) map[string]*BaseWallet {
	m := map[string]*BaseWallet{}
	_ = data.LoadJSON(file, &m)
	return m
}

// usable drops entries that decoded into nothing.
//
// A file written by something else parses cleanly into these structs and
// yields an entry per key with every field empty — which is how the credit
// ledger came to look like a full set of wallets holding no keys. An entry
// with neither an address nor a key is not a wallet, and treating the map as
// populated because it has keys in it is what stopped the fallback to the file
// that did have them.
func usable(m map[string]*BaseWallet) map[string]*BaseWallet {
	out := make(map[string]*BaseWallet, len(m))
	for id, w := range m {
		if w == nil || (w.Address == "" && w.PrivateKey == "") {
			continue
		}
		out[id] = w
	}
	return out
}

// BaseRPCURL returns the Base JSON-RPC endpoint: BASE_RPC_URL, or a public
// default that is at least on the right chain.
//
// TRADE_RPC_URL was a third option here, inherited from when the only thing
// reading a node was trading. It is gone, and the reason it had to go is the
// reason it was set: you trade on Ethereum, where the liquidity and the tokens
// are, and this instance's money is USDC on Base. So the fallback was not
// "might be another chain" — it was almost certainly another chain, and an
// Alchemy key is per-chain, so eth-mainnet and base-mainnet are different
// hostnames entirely.
//
// What that produced was silent: a Base token read against an Ethereum node
// finds no contract at the address, returns "0x", and hexToBigInt turns that
// into zero. Every wallet on the instance reads empty, with no error, forever.
// A public endpoint on the correct chain is worse for rate limits and better
// for being true.
func BaseRPCURL() string {
	if v := settings.Get("BASE_RPC_URL"); v != "" {
		return v
	}
	return "https://mainnet.base.org"
}

// expectedChain is the chain this instance's money is on, from the configured
// x402 network.
func expectedChain() int64 {
	if id, ok := chainIDFor(x402.Network()); ok {
		return id
	}
	return 8453
}

// chainName is the chain in words, for anything a person or a model reads.
//
// The number is what the protocol needs and says nothing to anybody else; "an
// address on 8453" is not an instruction somebody can follow when the thing
// that matters is not sending USDC on the wrong network.
func chainName() string {
	switch expectedChain() {
	case 8453:
		return "Base"
	case 84532:
		return "Base Sepolia"
	}
	return fmt.Sprintf("chain %d", expectedChain())
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
			"set BASE_RPC_URL to a node for chain %d", url, got, want, want)
	}
	return nil
}

// For returns a copy of the account's wallet, or nil if it has none yet.
//
// A copy, because the stored record is repaired in place — EnsureFor
// writes an address back onto a record that lost one — and a caller reading the
// key while that happens is an unsynchronised read of a private key. Nothing
// outside this file has any business writing one, and everything that does goes
// through a function here.
func For(accountID string) *BaseWallet {
	loadWallets()
	walletMu.RLock()
	defer walletMu.RUnlock()
	w, ok := userWallets[accountID]
	if !ok || w == nil {
		return nil
	}
	out := *w
	return &out
}

// EnsureFor returns the account's wallet, generating one on first use.
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
func EnsureFor(accountID string) (*BaseWallet, error) {
	loadWallets()
	walletMu.Lock()
	defer walletMu.Unlock()
	if w, ok := userWallets[accountID]; ok && w != nil {
		if w.Address != "" {
			return copyOf(w), nil
		}
		if addr, ok := AddressFromPrivateKeyHex(w.PrivateKey); ok {
			w.Address = addr
			saveWallets() //nolint:errcheck
			app.Log("wallet", "repaired the address on %s's wallet from its key", accountID)
			return copyOf(w), nil
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
	saveWallets() //nolint:errcheck
	return copyOf(w), nil
}

// copyOf hands out the values rather than the record they came from.
func copyOf(w *BaseWallet) *BaseWallet {
	out := *w
	return &out
}

// DeleteBaseWallet removes an account's on-chain wallet (account teardown).
func DeleteBaseWallet(accountID string) {
	loadWallets()
	walletMu.Lock()
	defer walletMu.Unlock()
	if _, ok := userWallets[accountID]; ok {
		delete(userWallets, accountID)
		// One fewer is the point of this function, so the guard is told to
		// expect it. Two fewer still is not.
		saveWalletsAllowing(1) //nolint:errcheck
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
	// One wrong value in BASE_RPC_URL does it, so this is a configuration away
	// rather than a theory.
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
			"chain (check BASE_RPC_URL)", BaseRPCURL(), token)
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
