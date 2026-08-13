package cli

// Paying for a call, from the command line, with a wallet on this machine.
//
// The payer has existed for a while and nothing could reach it. PayAndCallMCP
// takes a 402, signs an EIP-3009 authorisation from a Base wallet and retries —
// the whole mechanism — and it was wired to no tool, no page and no command. So
// an operator running an instance that advertises "agents pay per request in
// USDC" had no way to find out whether that was true, and neither did anybody
// evaluating it.
//
// This is the smallest thing that makes it real: a wallet you hold, a tool you
// name, and the 402 handled for you.
//
//	mu x402 call web_search query="x402"
//	mu x402 call --server https://micro.mu weather_forecast lat=51.5 lon=-0.12
//
// It is deliberately not a tool the agent can call. What spends money here is a
// person at a terminal with a key in a file, which is the one arrangement where
// "it paid without asking" is not a surprise.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mu/wallet"
)

// runX402Pay handles `mu x402 call ...`.
func runX402Pay(args []string) int {
	server := "https://micro.mu"
	seedPath := ""
	var rest []string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--server" && i+1 < len(args):
			server, i = args[i+1], i+1
		case strings.HasPrefix(args[i], "--server="):
			server = strings.TrimPrefix(args[i], "--server=")
		case args[i] == "--seed" && i+1 < len(args):
			seedPath, i = args[i+1], i+1
		case strings.HasPrefix(args[i], "--seed="):
			seedPath = strings.TrimPrefix(args[i], "--seed=")
		default:
			rest = append(rest, args[i])
		}
	}

	if len(rest) == 0 {
		fmt.Println("usage: mu x402 call [--server URL] [--seed PATH] <tool> [key=value ...]")
		fmt.Println()
		fmt.Println("  Calls a tool over MCP with no credentials. If the server answers 402,")
		fmt.Println("  signs a USDC payment from the wallet in --seed and retries.")
		fmt.Println()
		fmt.Println("  mu x402 call web_search query=\"x402\"")
		return 2
	}

	tool, pairs := rest[0], rest[1:]
	toolArgs := map[string]any{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			fmt.Printf("argument %q is not key=value\n", p)
			return 2
		}
		toolArgs[k] = v
	}

	// The wallet is optional, because most of the catalogue is free and calling
	// a free tool with no credentials and no key is the clearest demonstration
	// there is of what this endpoint offers. It is only needed if the server
	// answers 402, and the error then says so.
	bw, err := walletFromSeed(seedPath)
	if err != nil {
		fmt.Printf("no wallet (%v) — free tools will still answer\n", err)
	} else {
		fmt.Printf("paying from %s if asked\n", bw.Address)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// No account id: this is the anonymous path, which is the entire point —
	// the caller has no login here and the payment is the identity. The daily
	// cap that argument meters is the instance's own, and there is no instance.
	out, err := wallet.PayAndCallMCP(ctx, "", server, tool, toolArgs, bw)
	if err != nil {
		fmt.Println("failed:", err)
		if bw == nil && strings.Contains(err.Error(), "no wallet") {
			fmt.Println()
			fmt.Println("That tool is priced. Put a funded Base wallet's key in")
			fmt.Println("~/.mu/keys/wallet.seed, or pass --seed. `mu wallet` says which")
			fmt.Println("address a seed controls.")
		}
		return 1
	}
	fmt.Println(out)
	return 0
}

// walletFromSeed loads a Base wallet from a private key on disk.
//
// The same file `mu wallet` audits, so an operator who has already checked
// which address they control pays from that one. Read here rather than through
// GetOrCreateWallet, because that keys wallets by account and there is no
// account in this flow.
func walletFromSeed(path string) (*wallet.BaseWallet, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".mu", "keys", "wallet.seed")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}
	key := strings.TrimPrefix(strings.TrimSpace(string(raw)), "0x")
	addr, ok := wallet.AddressFromPrivateKeyHex(key)
	if !ok {
		return nil, fmt.Errorf("%s does not hold a 32-byte hex private key", path)
	}
	return &wallet.BaseWallet{Address: addr, PrivateKey: key}, nil
}
