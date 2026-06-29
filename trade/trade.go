// Package trade provides DEX trading via Uniswap V3 on Base.
// Each user gets an on-chain wallet (generated or imported).
// The agent can execute swaps on behalf of the user.
package trade

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/data"
	"mu/internal/settings"

	"golang.org/x/crypto/sha3"
)

// Chain configuration. Defaults to Ethereum mainnet.
// Set TRADE_CHAIN=base for Base L2.
type ChainConfig struct {
	Name     string
	ChainID  int64
	RPCURL   string
	WETH     string
	USDC     string
	Router   string
	Quoter   string
	Explorer string
}

var chains = map[string]ChainConfig{
	"ethereum": {
		Name:     "Ethereum",
		ChainID:  1,
		RPCURL:   "https://eth.llamarpc.com",
		WETH:     "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		USDC:     "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Router:   "0xE592427A0AEce92De3Edee1F18E0157C05861564",
		Quoter:   "0x61fFE014bA17989E743c5F6cB21bF9697530B21e",
		Explorer: "https://etherscan.io",
	},
	"base": {
		Name:     "Base",
		ChainID:  8453,
		RPCURL:   "https://mainnet.base.org",
		WETH:     "0x4200000000000000000000000000000000000006",
		USDC:     "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		Router:   "0x2626664c2603336E57B271c5C0b26F421741e481",
		Quoter:   "0x3d4e44Eb1374240CE5F1B871ab261CD16335B76a",
		Explorer: "https://basescan.org",
	},
}

func activeChain() ChainConfig {
	name := settings.Get("TRADE_CHAIN")
	if name == "" {
		name = "ethereum"
	}
	if c, ok := chains[name]; ok {
		return c
	}
	return chains["ethereum"]
}

func ActiveChainName() string { return activeChain().Name }
func ChainExplorer() string   { return activeChain().Explorer }

var Tokens map[string]Token
var tokenOrder []string

func initTokens() {
	c := activeChain()

	// Common tokens shared across chains
	Tokens = map[string]Token{
		"ETH":  {Symbol: "ETH", Name: "Ether", Decimals: 18, Address: "0x0000000000000000000000000000000000000000", Native: true},
		"WETH": {Symbol: "WETH", Name: "Wrapped Ether", Decimals: 18, Address: c.WETH},
		"USDC": {Symbol: "USDC", Name: "USD Coin", Decimals: 6, Address: c.USDC},
	}

	// Chain-specific tokens
	if c.ChainID == 1 { // Ethereum mainnet
		Tokens["USDT"] = Token{Symbol: "USDT", Name: "Tether", Decimals: 6, Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7"}
		Tokens["DAI"] = Token{Symbol: "DAI", Name: "Dai", Decimals: 18, Address: "0x6B175474E89094C44Da98b954EedeAC495271d0F"}
		Tokens["WBTC"] = Token{Symbol: "WBTC", Name: "Wrapped Bitcoin", Decimals: 8, Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599"}
		Tokens["UNI"] = Token{Symbol: "UNI", Name: "Uniswap", Decimals: 18, Address: "0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984"}
		Tokens["LINK"] = Token{Symbol: "LINK", Name: "Chainlink", Decimals: 18, Address: "0x514910771AF9Ca656af840dff83E8264EcF986CA"}
		Tokens["AAVE"] = Token{Symbol: "AAVE", Name: "Aave", Decimals: 18, Address: "0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9"}
		Tokens["MKR"] = Token{Symbol: "MKR", Name: "Maker", Decimals: 18, Address: "0x9f8F72aA9304c8B593d555F12eF6589cC3A579A2"}
	} else if c.ChainID == 8453 { // Base
		Tokens["DAI"] = Token{Symbol: "DAI", Name: "Dai", Decimals: 18, Address: "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb"}
		Tokens["cbETH"] = Token{Symbol: "cbETH", Name: "Coinbase ETH", Decimals: 18, Address: "0x2Ae3F1Ec7F1F5012CFEab0185bfc7aa3cf0DEc22"}
	}

	tokenOrder = []string{"ETH", "USDC", "USDT", "DAI", "WBTC", "UNI", "LINK", "AAVE", "MKR", "WETH", "cbETH"}
	// Filter to only tokens that exist on this chain
	var filtered []string
	for _, s := range tokenOrder {
		if _, ok := Tokens[s]; ok {
			filtered = append(filtered, s)
		}
	}
	tokenOrder = filtered
}

func UniswapRouterAddr() string { return activeChain().Router }
func UniswapQuoterAddr() string { return activeChain().Quoter }

type Token struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals int    `json:"decimals"`
	Address  string `json:"address"`
	Native   bool   `json:"native,omitempty"`
}

// Wallet holds a user's on-chain trading wallet.
type Wallet struct {
	Address    string `json:"address"`
	PrivateKey string `json:"private_key"` // hex-encoded, stored encrypted at rest
}

// Trade records a completed or pending swap.
type Trade struct {
	ID        string `json:"id"`
	Account   string `json:"account"`
	FromToken string `json:"from_token"`
	ToToken   string `json:"to_token"`
	AmountIn  string `json:"amount_in"`
	AmountOut string `json:"amount_out"`
	TxHash    string `json:"tx_hash,omitempty"`
	Status    string `json:"status"` // pending, confirmed, failed
	CreatedAt string `json:"created_at"`
	GasUsed   string `json:"gas_used,omitempty"`
}

var (
	walletMu sync.RWMutex
	wallets  = map[string]*Wallet{}  // accountID → wallet
	trades   = map[string][]*Trade{} // accountID → trades
)

func Load() {
	initTokens()
	if err := service.Register("trade", new(Server)); err != nil {
		app.Log("trade", "service register failed: %v", err)
	}
	data.LoadJSON("trade_wallets.json", &wallets)
	data.LoadJSON("trade_history.json", &trades)
	loadStrategies()
	StartSignalLoop()
	app.Log("trade", "Trading on %s (chain ID %d)", activeChain().Name, activeChain().ChainID)
}

// Enabled returns true. Trading uses Base mainnet by default.
func Enabled() bool {
	return true
}

func rpcURL() string {
	if v := settings.Get("TRADE_RPC_URL"); v != "" {
		return v
	}
	return activeChain().RPCURL
}

// GetWallet returns the trading wallet for a user, or nil.
func GetWallet(accountID string) *Wallet {
	walletMu.RLock()
	defer walletMu.RUnlock()
	return wallets[accountID]
}

// CreateWallet generates a new Ethereum keypair for the user.
func CreateWallet(accountID string) (*Wallet, error) {
	walletMu.Lock()
	defer walletMu.Unlock()

	if w, ok := wallets[accountID]; ok {
		return w, nil
	}

	privKey, addr, err := generateKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	w := &Wallet{
		Address:    addr,
		PrivateKey: privKey,
	}
	wallets[accountID] = w
	data.SaveJSON("trade_wallets.json", wallets)
	return w, nil
}

// ImportWallet sets a user's trading wallet from an existing private key.
func ImportWallet(accountID, privateKeyHex string) (*Wallet, error) {
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")
	if len(privateKeyHex) != 64 {
		return nil, errors.New("invalid private key length")
	}

	keyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, errors.New("invalid hex in private key")
	}

	addr := addressFromPrivateKey(keyBytes)

	walletMu.Lock()
	defer walletMu.Unlock()

	w := &Wallet{
		Address:    addr,
		PrivateKey: privateKeyHex,
	}
	wallets[accountID] = w
	data.SaveJSON("trade_wallets.json", wallets)
	return w, nil
}

// GetTrades returns recent trades for a user.
func GetTrades(accountID string, limit int) []*Trade {
	walletMu.RLock()
	defer walletMu.RUnlock()

	if limit <= 0 {
		return nil
	}

	t := trades[accountID]
	if len(t) <= limit {
		return t
	}
	return t[len(t)-limit:]
}

func saveTrade(accountID string, t *Trade) {
	walletMu.Lock()
	defer walletMu.Unlock()
	trades[accountID] = append(trades[accountID], t)
	if len(trades[accountID]) > 100 {
		trades[accountID] = trades[accountID][len(trades[accountID])-100:]
	}
	data.SaveJSON("trade_history.json", trades)
}

// ParseAmount converts a human-readable amount (e.g. "100") to the
// token's smallest unit based on decimals.
func ParseAmount(amount string, decimals int) (*big.Int, error) {
	if decimals < 0 {
		return nil, errors.New("invalid decimals")
	}

	amount = strings.TrimSpace(amount)
	parts := strings.Split(amount, ".")
	if len(parts) > 2 {
		return nil, errors.New("invalid amount")
	}

	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if whole == "" && frac == "" {
		return nil, errors.New("invalid amount")
	}
	if !allDigits(whole) || !allDigits(frac) {
		return nil, errors.New("invalid number")
	}

	if len(frac) > decimals {
		return nil, errors.New("too many decimal places")
	}
	for len(frac) < decimals {
		frac += "0"
	}

	combined := whole + frac
	if combined == "" {
		combined = "0"
	}
	val, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return nil, errors.New("invalid number")
	}
	return val, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// FormatAmount converts a raw token amount to human-readable form.
func FormatAmount(raw *big.Int, decimals int) string {
	if raw == nil {
		return "0"
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(raw, divisor)
	frac := new(big.Int).Mod(raw, divisor)

	fracStr := fmt.Sprintf("%0*s", decimals, frac.String())
	fracStr = strings.TrimRight(fracStr, "0")
	if fracStr == "" {
		return whole.String()
	}
	if len(fracStr) > 6 {
		fracStr = fracStr[:6]
	}
	return whole.String() + "." + fracStr
}

// ── Key generation (pure Go, no go-ethereum dependency) ──

func generateKeypair() (privKeyHex, address string, err error) {
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return "", "", err
	}
	privKeyHex = hex.EncodeToString(key)
	address = addressFromPrivateKey(key)
	return privKeyHex, address, nil
}

// addressFromPrivateKey derives an Ethereum address from a 32-byte private key
// using secp256k1 curve multiplication and Keccak-256 hashing.
func addressFromPrivateKey(privKey []byte) string {
	pubKey := secp256k1PublicKey(privKey)
	hash := keccak256(pubKey[1:]) // skip 0x04 prefix
	return "0x" + hex.EncodeToString(hash[12:])
}

func keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// secp256k1PublicKey computes the uncompressed public key from a private key.
// Uses the secp256k1 curve: y² = x³ + 7 over F_p.
func secp256k1PublicKey(privKey []byte) []byte {
	// secp256k1 curve parameters
	p, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	gx, _ := new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	gy, _ := new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	n, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)

	k := new(big.Int).SetBytes(privKey)
	k.Mod(k, n)

	// Scalar multiplication: Q = k * G
	rx, ry := scalarMult(gx, gy, k, p)

	xBytes := rx.Bytes()
	yBytes := ry.Bytes()

	// Pad to 32 bytes
	pub := make([]byte, 65)
	pub[0] = 0x04
	copy(pub[1+32-len(xBytes):33], xBytes)
	copy(pub[33+32-len(yBytes):65], yBytes)

	return pub
}

// Elliptic curve scalar multiplication using double-and-add.
func scalarMult(gx, gy, k, p *big.Int) (*big.Int, *big.Int) {
	rx, ry := new(big.Int), new(big.Int)
	isZero := true

	for i := k.BitLen() - 1; i >= 0; i-- {
		if !isZero {
			rx, ry = pointDouble(rx, ry, p)
		}
		if k.Bit(i) == 1 {
			if isZero {
				rx.Set(gx)
				ry.Set(gy)
				isZero = false
			} else {
				rx, ry = pointAdd(rx, ry, gx, gy, p)
			}
		}
	}
	return rx, ry
}

func pointAdd(x1, y1, x2, y2, p *big.Int) (*big.Int, *big.Int) {
	dy := new(big.Int).Sub(y2, y1)
	dx := new(big.Int).Sub(x2, x1)
	dx.ModInverse(dx, p)
	s := new(big.Int).Mul(dy, dx)
	s.Mod(s, p)

	rx := new(big.Int).Mul(s, s)
	rx.Sub(rx, x1)
	rx.Sub(rx, x2)
	rx.Mod(rx, p)

	ry := new(big.Int).Sub(x1, rx)
	ry.Mul(ry, s)
	ry.Sub(ry, y1)
	ry.Mod(ry, p)

	return rx, ry
}

func pointDouble(x, y, p *big.Int) (*big.Int, *big.Int) {
	three := big.NewInt(3)
	two := big.NewInt(2)

	x2 := new(big.Int).Mul(x, x)
	num := new(big.Int).Mul(three, x2)
	den := new(big.Int).Mul(two, y)
	den.ModInverse(den, p)
	s := new(big.Int).Mul(num, den)
	s.Mod(s, p)

	rx := new(big.Int).Mul(s, s)
	rx.Sub(rx, new(big.Int).Mul(two, x))
	rx.Mod(rx, p)

	ry := new(big.Int).Sub(x, rx)
	ry.Mul(ry, s)
	ry.Sub(ry, y)
	ry.Mod(ry, p)

	return rx, ry
}

// ── JSON-RPC helpers for Base chain interaction ──

// TokenInfo is returned by the API for available tokens.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}

func ListTokens() []TokenInfo {
	var out []TokenInfo
	for _, t := range Tokens {
		out = append(out, TokenInfo{
			Symbol:   t.Symbol,
			Name:     t.Name,
			Address:  t.Address,
			Decimals: t.Decimals,
		})
	}
	return out
}

// DeleteWallet removes a user's trading wallet (for account deletion).
func DeleteWallet(accountID string) {
	walletMu.Lock()
	defer walletMu.Unlock()
	delete(wallets, accountID)
	delete(trades, accountID)
	data.SaveJSON("trade_wallets.json", wallets)
	data.SaveJSON("trade_history.json", trades)
}

// WalletInfo is the public view of a user's trading wallet.
type WalletInfo struct {
	Address string `json:"address"`
}

// GetWalletInfo returns the public wallet info (no private key).
func GetWalletInfo(accountID string) *WalletInfo {
	w := GetWallet(accountID)
	if w == nil {
		return nil
	}
	return &WalletInfo{Address: w.Address}
}

// RecheckTrades looks up any pending/failed trades with a tx hash
// and corrects their status from on-chain data.
func RecheckTrades(accountID string) int {
	walletMu.Lock()
	defer walletMu.Unlock()

	fixed := 0
	for _, t := range trades[accountID] {
		if t.TxHash == "" || t.Status == "confirmed" {
			continue
		}
		gasUsed, err := waitForReceiptOnce(t.TxHash)
		if err != nil {
			continue
		}
		t.Status = "confirmed"
		t.GasUsed = gasUsed
		fixed++
	}
	if fixed > 0 {
		data.SaveJSON("trade_history.json", trades)
	}
	return fixed
}

var _ = json.Marshal
