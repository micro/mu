package cli

import (
	"fmt"

	"mu/wallet"
)

// runX402 prints the x402 configuration and, when CDP credentials are present
// in the environment, certifies facilitator auth by minting a Bearer JWT and
// probing the supported schemes/networks. It reads credentials locally and
// never prints the secret.
//
//	mu x402
func runX402(args []string) int {
	fmt.Print(wallet.X402Status())
	return 0
}
