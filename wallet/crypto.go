package wallet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/sha3"

	"mu/app"
	"mu/data"
)

var (
	masterKey   *bip32.Key
	cryptoMutex sync.RWMutex
	seedLoaded  bool

	// Deposit detection
	baseRPCURL         = getEnvOrDefault("BASE_RPC_URL", "https://mainnet.base.org")
	depositPollSecs    = getEnvInt("DEPOSIT_POLL_INTERVAL", 30)
	lastProcessedBlock = make(map[string]uint64)
	depositMutex       sync.Mutex
)

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// InitCryptoWallet initializes the HD wallet from seed
// Checks WALLET_SEED env var first, then ~/.mu/keys/wallet.seed file
// If neither exists, generates new seed and saves to file
func InitCryptoWallet() error {
	cryptoMutex.Lock()
	defer cryptoMutex.Unlock()

	if seedLoaded {
		return nil
	}

	var mnemonic string

	// 1. Check env var
	if seed := os.Getenv("WALLET_SEED"); seed != "" {
		mnemonic = strings.TrimSpace(seed)
		app.Log("wallet", "Using seed from WALLET_SEED env var")
	} else {
		// 2. Check file
		seedPath := getSeedPath()
		if fileData, err := os.ReadFile(seedPath); err == nil {
			mnemonic = strings.TrimSpace(string(fileData))
			app.Log("wallet", "Loaded seed from %s", seedPath)
		} else if os.IsNotExist(err) {
			// 3. Generate new seed
			entropy, err := bip39.NewEntropy(256) // 24 words
			if err != nil {
				return fmt.Errorf("failed to generate entropy: %w", err)
			}
			mnemonic, err = bip39.NewMnemonic(entropy)
			if err != nil {
				return fmt.Errorf("failed to generate mnemonic: %w", err)
			}

			// Save to file
			if err := saveSeed(seedPath, mnemonic); err != nil {
				return fmt.Errorf("failed to save seed: %w", err)
			}
			app.Log("wallet", "Generated new wallet seed - BACK THIS UP: %s", seedPath)
		} else {
			return fmt.Errorf("failed to read seed file: %w", err)
		}
	}

	// Validate mnemonic
	if !bip39.IsMnemonicValid(mnemonic) {
		return errors.New("invalid mnemonic")
	}

	// Derive master key from seed
	seed := bip39.NewSeed(mnemonic, "") // No passphrase
	var err error
	masterKey, err = bip32.NewMasterKey(seed)
	if err != nil {
		return fmt.Errorf("failed to create master key: %w", err)
	}

	seedLoaded = true

	// Log treasury address (index 0) - do in goroutine to not block startup
	go func() {
		defer func() {
			if r := recover(); r != nil {
				app.Log("wallet", "Error deriving treasury address: %v", r)
			}
		}()
		addr, err := DeriveAddress(0)
		if err != nil {
			app.Log("wallet", "Failed to derive treasury address: %v", err)
		} else {
			app.Log("wallet", "Treasury address (index 0): %s", addr)
		}
	}()

	return nil
}

// getSeedPath returns the path to the wallet seed file
func getSeedPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "wallet.seed" // Fallback to current dir
	}
	return filepath.Join(homeDir, ".mu", "keys", "wallet.seed")
}

// saveSeed saves the mnemonic to file with restrictive permissions
func saveSeed(path, mnemonic string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write with restrictive permissions (owner read/write only)
	return os.WriteFile(path, []byte(mnemonic+"\n"), 0600)
}

// DeriveAddress derives an Ethereum address for a given index
// Uses BIP44 path: m/44'/60'/0'/0/{index}
func DeriveAddress(index uint32) (string, error) {
	cryptoMutex.RLock()
	defer cryptoMutex.RUnlock()

	if masterKey == nil {
		return "", errors.New("wallet not initialized")
	}

	// BIP44 derivation path for Ethereum: m/44'/60'/0'/0/{index}
	purpose, err := masterKey.NewChildKey(bip32.FirstHardenedChild + 44)
	if err != nil {
		return "", fmt.Errorf("failed to derive purpose: %w", err)
	}

	coinType, err := purpose.NewChildKey(bip32.FirstHardenedChild + 60)
	if err != nil {
		return "", fmt.Errorf("failed to derive coin type: %w", err)
	}

	account, err := coinType.NewChildKey(bip32.FirstHardenedChild + 0)
	if err != nil {
		return "", fmt.Errorf("failed to derive account: %w", err)
	}

	change, err := account.NewChildKey(0)
	if err != nil {
		return "", fmt.Errorf("failed to derive change: %w", err)
	}

	addressKey, err := change.NewChildKey(index)
	if err != nil {
		return "", fmt.Errorf("failed to derive address key: %w", err)
	}

	return compressedPubKeyToAddress(addressKey.PublicKey().Key), nil
}

// compressedPubKeyToAddress converts a compressed public key to an Ethereum address
func compressedPubKeyToAddress(compressedPubKey []byte) string {
	pubKey, err := btcec.ParsePubKey(compressedPubKey)
	if err != nil {
		return ""
	}

	// Get uncompressed public key bytes (65 bytes: 0x04 + X + Y)
	uncompressed := pubKey.SerializeUncompressed()

	// Remove the 0x04 prefix, keep only X and Y (64 bytes)
	pubKeyBytes := uncompressed[1:]

	// Keccak256 hash of public key
	hash := keccak256(pubKeyBytes)

	// Take last 20 bytes as address
	address := hash[len(hash)-20:]

	return "0x" + hex.EncodeToString(address)
}

// keccak256 computes the Keccak256 hash
func keccak256(d []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(d)
	return hash.Sum(nil)
}

// GetUserDepositAddress gets or creates a deposit address for a user
func GetUserDepositAddress(userID string) (string, error) {
	if err := InitCryptoWallet(); err != nil {
		return "", err
	}
	index := getUserAddressIndex(userID)
	return DeriveAddress(index)
}

// getUserAddressIndex returns the BIP32 index for a user
// Index 0 is reserved for treasury, users start at 1
func getUserAddressIndex(userID string) uint32 {
	hash := keccak256([]byte(userID))
	index := uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
	index = (index % 2147483646) + 1 // Ensure > 0 and < 2^31
	return index
}

// GetTreasuryAddress returns the main treasury address (index 0)
func GetTreasuryAddress() (string, error) {
	if err := InitCryptoWallet(); err != nil {
		return "", err
	}
	return DeriveAddress(0)
}

// CryptoWalletEnabled returns true if the crypto wallet is available
func CryptoWalletEnabled() bool {
	return seedLoaded || os.Getenv("WALLET_SEED") != "" || fileExists(getSeedPath())
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ============================================
// DEPOSIT DETECTION (Phase 2)
// ============================================

// CryptoDeposit represents a detected deposit
type CryptoDeposit struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	TxHash      string    `json:"tx_hash"`
	Token       string    `json:"token"`        // "ETH" or contract address
	TokenSymbol string    `json:"token_symbol"` // "ETH", "USDC", etc.
	Amount      *big.Int  `json:"amount"`       // Raw amount
	AmountUSD   float64   `json:"amount_usd"`   // USD value at deposit time
	Credits     int       `json:"credits"`      // Credits awarded
	BlockNumber uint64    `json:"block_number"`
	CreatedAt   time.Time `json:"created_at"`
}

// Known tokens on Base
var knownTokens = map[string]struct {
	Symbol   string
	Decimals int
}{
	"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913": {"USDC", 6},
	"0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb": {"DAI", 18},
	"0x4200000000000000000000000000000000000006": {"WETH", 18},
}

// StartDepositWatcher starts the background deposit detection
func StartDepositWatcher() {
	if err := InitCryptoWallet(); err != nil {
		app.Log("wallet", "Cannot start deposit watcher: %v", err)
		return
	}

	go func() {
		app.Log("wallet", "Deposit watcher started (polling every %ds)", depositPollSecs)

		ticker := time.NewTicker(time.Duration(depositPollSecs) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			checkForDeposits()
		}
	}()
}

// Track ETH balances for deposit detection
var (
	ethBalances     = make(map[string]*big.Int)
	ethBalanceMutex sync.RWMutex
)

// checkForDeposits checks all user addresses for new deposits
func checkForDeposits() {
	currentBlock, err := getBlockNumber()
	if err != nil {
		app.Log("wallet", "Failed to get block number: %v", err)
		return
	}

	// Get all users with wallets
	mutex.RLock()
	userIDs := make([]string, 0, len(wallets))
	for userID := range wallets {
		userIDs = append(userIDs, userID)
	}
	mutex.RUnlock()

	for _, userID := range userIDs {
		addr, err := GetUserDepositAddress(userID)
		if err != nil {
			continue
		}

		// Check native ETH deposits
		checkETHDeposit(userID, addr)

		// Check ERC-20 deposits
		checkERC20Deposits(userID, addr, currentBlock)
	}
}

// checkETHDeposit detects native ETH deposits by comparing balance
func checkETHDeposit(userID, addr string) {
	newBalance, err := getETHBalance(addr)
	if err != nil || newBalance == nil {
		return
	}

	ethBalanceMutex.RLock()
	oldBalance, exists := ethBalances[addr]
	ethBalanceMutex.RUnlock()

	if !exists {
		// First time seeing this address, just record balance
		ethBalanceMutex.Lock()
		ethBalances[addr] = newBalance
		ethBalanceMutex.Unlock()
		return
	}

	// Check if balance increased
	if newBalance.Cmp(oldBalance) > 0 {
		deposit := new(big.Int).Sub(newBalance, oldBalance)

		// Get USD value
		usdValue := getTokenUSDValue("ETH", deposit)
		credits := int(usdValue * 100)

		if credits >= 1 {
			// Generate a unique ID for this deposit
			depositID := fmt.Sprintf("eth_%s_%d", addr, time.Now().UnixNano())

			// Check not already processed
			key := "deposit_" + depositID
			if _, err := data.LoadFile(key); err == nil {
				return
			}

			// Add credits
			err := AddCredits(userID, credits, OpTopup, map[string]interface{}{
				"type":       "ETH",
				"amount":     deposit.String(),
				"amount_usd": usdValue,
			})
			if err != nil {
				app.Log("wallet", "Failed to add ETH credits: %v", err)
				return
			}

			// Mark as processed
			data.SaveJSON(key, map[string]interface{}{
				"user_id":    userID,
				"amount":     deposit.String(),
				"amount_usd": usdValue,
				"credits":    credits,
				"created_at": time.Now(),
			})

			app.Log("wallet", "Credited %d credits to %s for ETH deposit: %s ($%.2f)",
				credits, userID, deposit.String(), usdValue)
		}
	}

	// Update stored balance
	ethBalanceMutex.Lock()
	ethBalances[addr] = newBalance
	ethBalanceMutex.Unlock()
}

// getETHBalance gets ETH balance for an address
func getETHBalance(addr string) (*big.Int, error) {
	resp, err := rpcCall("eth_getBalance", []interface{}{addr, "latest"})
	if err != nil {
		return nil, err
	}

	var result string
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	if len(result) < 3 {
		return big.NewInt(0), nil
	}

	balance, _ := new(big.Int).SetString(result[2:], 16)
	return balance, nil
}

// getBlockNumber gets the current block number from Base RPC
func getBlockNumber() (uint64, error) {
	resp, err := rpcCall("eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var result string
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, err
	}

	if len(result) < 3 {
		return 0, errors.New("invalid block number")
	}

	block, _ := new(big.Int).SetString(result[2:], 16)
	return block.Uint64(), nil
}

// checkERC20Deposits checks for ERC-20 token deposits
func checkERC20Deposits(userID, addr string, currentBlock uint64) {
	depositMutex.Lock()
	lastBlock := lastProcessedBlock[addr]
	depositMutex.Unlock()

	if lastBlock == 0 {
		// First time - start from current block
		depositMutex.Lock()
		lastProcessedBlock[addr] = currentBlock
		depositMutex.Unlock()
		return
	}

	if currentBlock <= lastBlock {
		return
	}

	// Query Transfer events where 'to' is this address
	// Topic0: Transfer(address,address,uint256) = 0xddf252ad...
	transferTopic := "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	toTopic := "0x000000000000000000000000" + strings.ToLower(addr[2:]) // Pad address to 32 bytes

	logs, err := getLogs(lastBlock+1, currentBlock, transferTopic, "", toTopic)
	if err != nil {
		app.Log("wallet", "Failed to get ERC20 logs for %s: %v", addr, err)
		return
	}

	for _, log := range logs {
		dep := parseERC20Transfer(log)
		if dep != nil {
			dep.UserID = userID
			processDeposit(userID, dep)
		}
	}

	depositMutex.Lock()
	lastProcessedBlock[addr] = currentBlock
	depositMutex.Unlock()
}

// getLogs gets logs matching the filter
func getLogs(fromBlock, toBlock uint64, topics ...string) ([]map[string]interface{}, error) {
	topicsArray := make([]interface{}, len(topics))
	for i, t := range topics {
		if t == "" {
			topicsArray[i] = nil
		} else {
			topicsArray[i] = t
		}
	}

	params := map[string]interface{}{
		"fromBlock": fmt.Sprintf("0x%x", fromBlock),
		"toBlock":   fmt.Sprintf("0x%x", toBlock),
		"topics":    topicsArray,
	}

	resp, err := rpcCall("eth_getLogs", []interface{}{params})
	if err != nil {
		return nil, err
	}

	var logs []map[string]interface{}
	if err := json.Unmarshal(resp, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}

// parseERC20Transfer parses a Transfer event log
func parseERC20Transfer(log map[string]interface{}) *CryptoDeposit {
	topics, ok := log["topics"].([]interface{})
	if !ok || len(topics) < 3 {
		return nil
	}

	dataStr, ok := log["data"].(string)
	if !ok || len(dataStr) < 3 {
		return nil
	}

	txHash, _ := log["transactionHash"].(string)
	blockNumHex, _ := log["blockNumber"].(string)
	contractAddr, _ := log["address"].(string)

	// Parse amount from data
	amount, _ := new(big.Int).SetString(dataStr[2:], 16)

	// Parse block number
	blockNum := uint64(0)
	if len(blockNumHex) > 2 {
		bn, _ := new(big.Int).SetString(blockNumHex[2:], 16)
		blockNum = bn.Uint64()
	}

	// Get token info
	tokenSymbol := "UNKNOWN"
	if info, ok := knownTokens[contractAddr]; ok {
		tokenSymbol = info.Symbol
	}

	return &CryptoDeposit{
		ID:          txHash,
		TxHash:      txHash,
		Token:       contractAddr,
		TokenSymbol: tokenSymbol,
		Amount:      amount,
		BlockNumber: blockNum,
		CreatedAt:   time.Now(),
	}
}

// processDeposit processes a detected deposit and credits the user
func processDeposit(userID string, dep *CryptoDeposit) {
	if dep == nil || dep.TxHash == "" {
		return
	}

	// Check if already processed
	key := "deposit_" + dep.TxHash
	if _, err := data.LoadFile(key); err == nil {
		return // Already processed
	}

	// Get USD value
	usdValue := getTokenUSDValue(dep.TokenSymbol, dep.Amount)
	dep.AmountUSD = usdValue

	// Convert to credits (1 credit = $0.01)
	credits := int(usdValue * 100)
	dep.Credits = credits

	if credits < 1 {
		app.Log("wallet", "Deposit too small to credit: %s %s ($%.4f)",
			dep.Amount.String(), dep.TokenSymbol, usdValue)
		return
	}

	// Add credits
	err := AddCredits(userID, credits, OpTopup, map[string]interface{}{
		"tx_hash":      dep.TxHash,
		"token":        dep.TokenSymbol,
		"amount":       dep.Amount.String(),
		"amount_usd":   dep.AmountUSD,
		"block_number": dep.BlockNumber,
	})
	if err != nil {
		app.Log("wallet", "Failed to add credits for deposit: %v", err)
		return
	}

	// Mark as processed
	data.SaveJSON(key, dep)

	app.Log("wallet", "Credited %d credits to %s for deposit: %s %s ($%.2f)",
		credits, userID, dep.Amount.String(), dep.TokenSymbol, usdValue)
}

// getTokenUSDValue gets the USD value of a token amount
func getTokenUSDValue(symbol string, amount *big.Int) float64 {
	if amount == nil {
		return 0
	}

	price := getTokenPrice(symbol)
	if price == 0 {
		return 0
	}

	// Get decimals
	decimals := 18
	for _, info := range knownTokens {
		if info.Symbol == symbol {
			decimals = info.Decimals
			break
		}
	}

	// Convert to float with proper decimals
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	amountFloat := new(big.Float).SetInt(amount)
	amountFloat.Quo(amountFloat, divisor)

	f, _ := amountFloat.Float64()
	return f * price
}

// Token price cache
var (
	tokenPriceCache     = make(map[string]float64)
	tokenPriceCacheTime = make(map[string]time.Time)
	priceCacheMutex     sync.RWMutex
)

// getTokenPrice gets current USD price for a token (cached for 5 min)
func getTokenPrice(symbol string) float64 {
	// Check cache
	priceCacheMutex.RLock()
	if price, ok := tokenPriceCache[symbol]; ok {
		if time.Since(tokenPriceCacheTime[symbol]) < 5*time.Minute {
			priceCacheMutex.RUnlock()
			return price
		}
	}
	priceCacheMutex.RUnlock()

	// Fetch from CoinGecko
	ids := map[string]string{
		"ETH":  "ethereum",
		"WETH": "ethereum",
		"USDC": "usd-coin",
		"DAI":  "dai",
	}

	id, ok := ids[symbol]
	if !ok {
		return 0
	}

	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", id)
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0
	}

	price := 0.0
	if d, ok := result[id]; ok {
		price = d["usd"]
	}

	// Update cache
	priceCacheMutex.Lock()
	tokenPriceCache[symbol] = price
	tokenPriceCacheTime[symbol] = time.Now()
	priceCacheMutex.Unlock()

	return price
}

// rpcCall makes a JSON-RPC call to the Base node
func rpcCall(method string, params []interface{}) (json.RawMessage, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(baseRPCURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, err
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
