package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mu/service/wallet"
)

// The key on this machine.
//
// This was `mu wallet`, and that word now means something else: a service on
// the server, with an address of its own and four tools to use it. Two
// different keys under one name — one the server holds for your account, one
// you hold and the server never sees — and the only thing telling them apart
// was which arguments you happened to type. That is not a distinction anybody
// should have to hold in their head about money.
//
// So this is `mu key`, which is what it is: a private key in a file, that this
// CLI signs with. The server has no idea it exists.
//
// The file it reads is still ~/.mu/keys/wallet.seed. A command can be renamed
// freely; a file that holds real money cannot, because the rename ships, the
// old path is never read again, and somebody's funds are at a key nothing
// looks for.

// KeyPath is where the signing key lives, unless a caller names another.
//
// Still wallet.seed. A command can be renamed freely; a file that holds real
// money cannot, because the rename ships, the old path is never read again, and
// somebody's funds sit at a key nothing looks for.
func KeyPath(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mu", "keys", "wallet.seed")
}

// KeyAddress returns the address a stored key controls.
//
// Shared with `mu x402`, which asks a different question about the same file:
// this one is "what do I sign with", that one is "is the address I am being
// paid at one I can prove I control". Same key, two people.
func KeyAddress(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	seed := strings.TrimSpace(string(raw))
	if compact := strings.TrimPrefix(seed, "0x"); len(compact) != 64 || !isHexStr(compact) {
		return "", false
	}
	return wallet.AddressFromPrivateKeyHex(seed)
}

// runKey says what this machine signs with, and what it holds.
//
//	mu key [path-to-key]   (default ~/.mu/keys/wallet.seed)
//
// Caller-side only. It used to also report whether the key controlled
// X402_PAY_TO, which is a different question asked by a different person: that
// is an operator checking they can reach the money their instance is being
// paid, and it lives in `mu x402` with the rest of what an operator configures.
// One command answering two people's questions is how each of them reads half
// an answer and stops.
func runKey(args []string) int {
	if len(args) > 0 && args[0] == "new" {
		return newKey(args[1:])
	}

	path := KeyPath(args)
	fmt.Println("key file:", path)

	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("key: could not read —", err)
		fmt.Println("Make one with `mu key new`.")
		return 1
	}
	seed := strings.TrimSpace(string(raw))
	compact := strings.TrimPrefix(seed, "0x")

	switch {
	case len(compact) == 64 && isHexStr(compact):
		addr, ok := wallet.AddressFromPrivateKeyHex(seed)
		if !ok {
			fmt.Println("key: not a valid private key")
			return 1
		}
		fmt.Println("address: ", addr)

		// What is actually in it. `mu key` could say which address a key
		// controlled and not what it held, which is the one question somebody
		// funding a wallet is asking — and the answer lived only in `mu agent`'s
		// startup line, where you had to start a session to see it.
		//
		// Read from the chain rather than from anything we store, because a
		// balance we remember is a balance that can be wrong.
		human, rawBal := wallet.USDCBalance(addr)
		switch {
		case rawBal == nil || rawBal.Sign() == 0:
			fmt.Println("balance:  0 USDC — send USDC on Base to the address above")
			fmt.Println("          (no ETH needed; you never pay gas)")
		default:
			fmt.Printf("balance:  %s USDC on Base\n", human)
		}
		fmt.Println()
		fmt.Println("This is what `mu agent` and `mu x402 call` sign with. It signs two")
		fmt.Println("things: payments, and who you are — free account-scoped tools need an")
		fmt.Println("identity and never charge for one.")
		return 0
	case len(strings.Fields(seed)) >= 12:
		fmt.Printf("key: looks like a %d-word mnemonic (not Mu's native raw-key format).\n", len(strings.Fields(seed)))
		fmt.Println("Verify on a trusted machine: import the mnemonic into a wallet and check")
		fmt.Println("which address it derives.")
		return 0
	default:
		fmt.Println("key: unrecognized format (neither a 64-hex private key nor a mnemonic).")
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

// newKey creates the key `mu agent` pays from.
//
// There was no way to make one. `mu key` audits one and the agent reads
// it, but nothing wrote it — so the honest answer to "how do I fund the wallet"
// was "generate a secp256k1 key by hand and put the hex in this file", which is
// not an answer. Running clean, the first thing anybody met was a path that did
// not exist and no way to make it exist.
func newKey(args []string) int {
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
		fmt.Println("Run `mu key` to see which address it controls.")
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
// Shared by `mu key new` and `mu agent`, which needs one on a clean run and
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
