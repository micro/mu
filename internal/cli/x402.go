package cli

import (
	"fmt"
	"os"
	"strings"

	"mu/internal/x402"
)

// runX402 prints the x402 configuration and, when CDP credentials are present
// in the environment, certifies facilitator auth by minting a Bearer JWT and
// probing the supported schemes/networks. It reads credentials locally and
// never prints the secret.
//
//	mu x402
func runX402(args []string) int {
	// `mu x402` alone reports configuration; `mu x402 call ...` actually pays.
	// The status was the whole command for a long time, which meant an operator
	// could confirm the endpoint was configured and never that it worked.
	if len(args) > 0 && args[0] == "call" {
		return runX402Pay(args[1:])
	}
	fmt.Print(x402.X402Status())

	// Can the operator actually reach what they are paid?
	//
	// X402_PAY_TO is where this instance's earnings land, and an address you
	// cannot prove you control is money you cannot spend — the failure is
	// silent right up until you try to move it. `mu key` used to answer this,
	// mixed in with what the caller signs with, so two different people read
	// one output and each took half of it.
	fmt.Println()
	fmt.Print(payToStatus())

	fmt.Println()
	fmt.Println("Pay for a call with the key on this machine:")
	fmt.Println("  mu x402 call web_search query=\"x402\"")
	fmt.Println("  mu key                    which key that is, and what it holds")
	return 0
}

// payToStatus reports whether a key on this machine controls the address this
// instance is paid at.
//
// Reads the key locally and prints only public addresses — never the key.
func payToStatus() string {
	payTo := strings.TrimSpace(os.Getenv("X402_PAY_TO"))
	if payTo == "" {
		payTo = dotenvValue("X402_PAY_TO")
	}
	if payTo == "" {
		return "pay-to control:  X402_PAY_TO is not set here, so there is nothing to check.\n" +
			"                 Run after `source ~/.env`, or pass it in the environment.\n"
	}

	path := KeyPath(nil)
	addr, ok := KeyAddress(path)
	if !ok {
		return fmt.Sprintf("pay-to control:  no usable key at %s, so this cannot be checked here.\n"+
			"                 Check it wherever the key for %s lives.\n", path, payTo)
	}
	if strings.EqualFold(addr, payTo) {
		return fmt.Sprintf("pay-to control:  ✓ %s controls %s.\n"+
			"                 Back that key up offline. It is the earnings.\n", path, payTo)
	}
	return fmt.Sprintf("pay-to control:  ✗ MISMATCH — %s controls %s, not %s.\n"+
		"                 USDC paid to this instance is held by a different key. Find it,\n"+
		"                 or point X402_PAY_TO at an address you can prove you control.\n",
		path, addr, payTo)
}
