package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mu/trade"
)

// runWallet audits which address a stored key controls and whether it matches
// the configured x402 pay-to address. It reads the seed locally and prints only
// public addresses — never the key itself.
//
//	mu wallet [path-to-seed]   (default ~/.mu/keys/wallet.seed)
func runWallet(args []string) int {
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
		addr, ok := trade.AddressFromPrivateKeyHex(seed)
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
