package cli

import (
	"fmt"

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
	fmt.Println()
	fmt.Println("Pay for a call from a wallet on this machine:")
	fmt.Println("  mu x402 call web_search query=\"x402\"")
	return 0
}
