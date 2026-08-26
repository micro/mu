// Package wallet is a key of your own: an address on Base, what it holds, and
// paying somebody else's priced endpoint with it.
//
// It was a top-level directory of the same name doing two unrelated jobs. One
// was the credit ledger — a balance an account tops up with a card — and that
// is the account's money and lives in account/ now. The other is this: a
// secp256k1 key, an address derived from it, and the signatures it can make.
// They shared a word and a directory, wrote the same file, and destroyed each
// other's data doing it.
//
// What is left is a service in the ordinary sense. It answers questions about
// state — what is my address, what does it hold — and it does one thing an
// agent cannot do without it: call a tool on a server that charges for it, sign
// the payment, and come back with the answer. No account with that server, no
// card on file, no key to rotate. That is the same claim this instance makes to
// its own callers, pointed outward.
//
// Two guards, and they are the service rather than decoration. Every payment is
// bounded per call and per day (spendlimit.go), because the agent reads text
// strangers wrote and a 402 challenge is text: a misled agent must not be able
// to empty the wallet no matter what a server claims its price is. And the
// source wallet is always the authenticated caller's own — a payment is never
// made from a wallet the caller did not prove they hold.
//
// Custody is the honest caveat. The key is on this instance's disk, which is
// what makes it usable out of the box on a self-hosted deployment and also
// means an operator who can read the disk can spend it. Hold what you are
// willing to have on a server.
package wallet

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the service handler. Its methods are exposed as RPC endpoints and,
// through the agent and gateways, as AI tools.
type Server struct{}

// caller is the account this call belongs to.
//
// There is no anonymous path here and there cannot be: every method either
// reads a key or spends one, and a key belongs to somebody. A caller who proved
// only a wallet signature is refused too — see AccountOnly on the endpoints —
// because "which account's wallet" has no answer for them.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use your wallet")
	}
	return id, nil
}

// ── Address ─────────────────────────────────────────────────────

type AddressRequest struct{}

type AddressResponse struct {
	Address string `json:"address" description:"The caller's address on Base"`
	Network string `json:"network" description:"Which chain the address is being used on"`
	Text    string `json:"text" description:"The address, said in a sentence"`
}

// Address returns the caller's wallet address, creating the wallet on first ask.
// @example {}
func (Server) Address(ctx context.Context, _ *AddressRequest, rsp *AddressResponse) error {
	id, err := caller(ctx)
	if err != nil {
		return err
	}
	bw, err := EnsureFor(id)
	if err != nil {
		return err
	}
	rsp.Address, rsp.Network = bw.Address, chainName()
	rsp.Text = fmt.Sprintf("Your address is %s, on %s. Only send funds on that network — "+
		"anything sent on another chain lands at the same address there, where this "+
		"instance cannot reach it.", bw.Address, rsp.Network)
	return nil
}

// ── Balance ─────────────────────────────────────────────────────

type BalanceRequest struct{}

type BalanceResponse struct {
	Address string `json:"address" description:"The address this balance belongs to"`
	USDC    string `json:"usdc" description:"USDC held, as a decimal string"`
	Network string `json:"network" description:"Which chain it was read from"`
	Text    string `json:"text" description:"What the wallet holds, said in a sentence"`
}

// Balance reports what the caller's wallet holds.
// @example {}
func (Server) Balance(ctx context.Context, _ *BalanceRequest, rsp *BalanceResponse) error {
	id, err := caller(ctx)
	if err != nil {
		return err
	}
	bw, err := EnsureFor(id)
	if err != nil {
		return err
	}
	// A balance that could not be read is not a balance of zero, and the two
	// mean opposite things to somebody who has just sent money: one says wait,
	// the other says go and look at the chain yourself.
	human, _, readErr := USDCBalanceErr(bw.Address)
	rsp.Address, rsp.Network = bw.Address, chainName()
	if readErr != nil {
		return fmt.Errorf("could not read the balance of %s on %s: %w", bw.Address, rsp.Network, readErr)
	}
	rsp.USDC = human
	rsp.Text = fmt.Sprintf("%s holds $%s USDC on %s.", bw.Address, human, rsp.Network)
	return nil
}

// No List, and no Pay.
//
// Pay called a tool on another server and settled the 402 if one came back,
// with List naming the servers it was allowed to call. Both are gone, and the
// name is why: wallet_pay reads as paying somebody, and what it did was make a
// tool call. Nobody reaches for a wallet endpoint to invoke a tool — they call
// the tool.
//
// And it is not the wallet that pays. A priced endpoint answers 402 and the
// client decides whether to settle it; the wallet holds the key and signs when
// asked. That is a runtime behaviour on a response code, not a capability a
// model picks off a list, and internal/cli already does it that way — see
// PayAndCallMCP, which stays because the CLI agent calls it on a 402.
//
// What is left is what a wallet is: an address, and what it holds.

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("wallet", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "wallet",
	Icon:        "wallet.png",
	Handler:     new(Server),
	Description: "A key of your own on Base: an address to hold USDC, and what it holds",
	Page:        "/wallet",
	// Every method here reads or spends somebody's key. There is no public half.
	Scoped: true,
	Endpoints: map[string]service.Endpoint{
		"Address": {
			Needs: service.Account,
			Doc: "Your own address on Base, to receive USDC. Created the first time you ask. " +
				"Funds sent on any other chain land at the same address there and cannot be reached from here",
		},
		"Balance": {
			Needs: service.Account,
			Doc: "What your wallet holds in USDC on Base. Says so plainly when the chain " +
				"could not be reached, because that is not the same as holding nothing",
		},
	},
}
