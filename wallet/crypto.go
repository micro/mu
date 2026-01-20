package wallet

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/sha3"

	"mu/app"
)

var (
	masterKey   *bip32.Key
	cryptoMutex sync.RWMutex
	seedLoaded  bool
)

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
		if data, err := os.ReadFile(seedPath); err == nil {
			mnemonic = strings.TrimSpace(string(data))
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

	// Log treasury address (index 0)
	addr, _ := DeriveAddress(0)
	app.Log("wallet", "Treasury address (index 0): %s", addr)

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
	// 44' = BIP44
	// 60' = Ethereum coin type
	// 0' = account
	// 0 = external chain
	// {index} = address index

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

	// Convert compressed public key to Ethereum address
	return compressedPubKeyToAddress(addressKey.PublicKey().Key), nil
}

// compressedPubKeyToAddress converts a compressed public key to an Ethereum address
func compressedPubKeyToAddress(compressedPubKey []byte) string {
	// Parse the compressed public key using btcec
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
func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

// GetUserDepositAddress gets or creates a deposit address for a user
func GetUserDepositAddress(userID string) (string, error) {
	// Ensure wallet is initialized
	if err := InitCryptoWallet(); err != nil {
		return "", err
	}

	// Get user's address index
	index := getUserAddressIndex(userID)

	return DeriveAddress(index)
}

// getUserAddressIndex returns the BIP32 index for a user
// Index 0 is reserved for treasury, users start at 1
func getUserAddressIndex(userID string) uint32 {
	// Hash userID to get deterministic index (1 to 2^31-1)
	// This ensures the same user always gets the same address
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
