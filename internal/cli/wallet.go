package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mu/wallet"
)

// runWallet audits which address a stored key controls and whether it matches
// the configured x402 pay-to address. It reads the seed locally and prints only
// public addresses — never the key itself.
//
//	mu wallet [path-to-seed]   (default ~/.mu/keys/wallet.seed)
func runWallet(args []string) int {
	if len(args) > 0 && args[0] == "new" {
		return newWallet(args[1:])
	}

	seedPath := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			seedPath = a
			break
		}
	}
	if seedPath == "" {
		home, _ := os.UserHomeDir()
		seedPath = filepath.Join(home, ".mu", "keys", "wallet.seed")
	}

	payTo := strings.TrimSpace(os.Getenv("X402_PAY_TO"))
	if payTo == "" {
		payTo = dotenvValue("X402_PAY_TO")
	}

	fmt.Println("x402 pay-to (X402_PAY_TO):", orNotSet(payTo))
	fmt.Println("seed file:", seedPath)

	raw, err := os.ReadFile(seedPath)
	if err != nil {
		fmt.Println("seed: could not read —", err)
		return 1
	}
	seed := strings.TrimSpace(string(raw))
	compact := strings.TrimPrefix(seed, "0x")

	switch {
	case len(compact) == 64 && isHexStr(compact):
		addr, ok := wallet.AddressFromPrivateKeyHex(seed)
		if !ok {
			fmt.Println("seed: not a valid private key")
			return 1
		}
		fmt.Println("seed controls address:", addr)
		if payTo == "" {
			fmt.Println("(X402_PAY_TO not set here — run after `source ~/.env`, or pass it in the environment)")
			return 0
		}
		if strings.EqualFold(addr, payTo) {
			fmt.Println("✓ MATCH — this seed controls your pay-to address. Back the seed up offline.")
			return 0
		}
		fmt.Println("✗ MISMATCH — the pay-to address is NOT controlled by this seed.")
		fmt.Println("  USDC sent to X402_PAY_TO is controlled by a different key. Find that key,")
		fmt.Println("  or point X402_PAY_TO at an address you can prove you control.")
		return 2
	case len(strings.Fields(seed)) >= 12:
		fmt.Printf("seed: looks like a %d-word mnemonic (not Mu's native raw-key format).\n", len(strings.Fields(seed)))
		fmt.Println("Verify on a trusted machine: import the mnemonic into a wallet and check its")
		fmt.Println("first address equals the pay-to address above.")
		return 0
	default:
		fmt.Println("seed: unrecognized format (neither a 64-hex private key nor a mnemonic).")
		return 1
	}
}

func isHexStr(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func orNotSet(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

// dotenvValue best-effort reads KEY from ~/.env when it isn't in the environment.
func dotenvValue(key string) string {
	home, _ := os.UserHomeDir()
	f, err := os.Open(filepath.Join(home, ".env"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "))
		if strings.HasPrefix(line, key+"=") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, key+"=")), `"'`)
		}
	}
	return ""
}

// newWallet creates the key `mu agent` pays from.
//
// There was no way to make one. `mu wallet` audits a seed and the agent reads
// it, but nothing wrote it — so the honest answer to "how do I fund the wallet"
// was "generate a secp256k1 key by hand and put the hex in this file", which is
// not an answer. Running clean, the first thing anybody met was a path that did
// not exist and no way to make it exist.
func newWallet(args []string) int {
	seedPath := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			seedPath = a
			break
		}
	}
	if seedPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("could not find your home directory:", err)
			return 1
		}
		seedPath = filepath.Join(home, ".mu", "keys", "wallet.seed")
	}

	// Never overwrite. The file being there means a key exists, and a key can
	// hold money — replacing it would strand whatever it holds, silently and
	// unrecoverably. Refusing is the only safe default.
	if _, err := os.Stat(seedPath); err == nil {
		fmt.Printf("%s already exists — not touching it.\n", seedPath)
		fmt.Println("Run `mu wallet` to see which address it controls.")
		return 1
	}

	addr, err := createSeed(seedPath)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	fmt.Println("address:", addr)
	fmt.Println("key:    ", seedPath, "(0600)")
	fmt.Println()
	fmt.Println("Send USDC on Base to that address to fund it. No ETH is needed —")
	fmt.Println("payments are signed here and the gas is paid by whoever settles them.")
	fmt.Println()
	fmt.Println("Back this file up. It is the only copy, and it is the money.")
	return 0
}

// createSeed writes a fresh key at path and returns its address.
//
// Shared by `mu wallet new` and `mu agent`, which needs one on a clean run and
// should not make somebody go and get it. Refuses to overwrite: the file being
// there means a key exists, a key can hold money, and replacing it would
// strand whatever it holds silently and unrecoverably.
func createSeed(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists — not touching it", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("could not create the key directory: %w", err)
	}
	priv, addr, err := wallet.GenerateKeypair()
	if err != nil {
		return "", fmt.Errorf("could not generate a key: %w", err)
	}
	if err := os.WriteFile(path, []byte(priv+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("could not write the key: %w", err)
	}
	return addr, nil
}
