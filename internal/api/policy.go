package api

// One place that answers what a tool costs and who may call it.
//
// Four questions were being asked in six places, and they disagreed:
//
//   - Does this need an account?  AccountOnly, OptionalAuth, HandleAuth,
//     MCPToolNeedsAuth, and whatever the service's own HTTP handler decided.
//   - Does it cost anything?      Endpoint.Cost → Tool.WalletOp →
//     GetOperationCost, read as "has an operation" in one place and "costs
//     something" in another.
//   - Can this caller pay?        api.QuotaCheck, agent.QuotaCheck, the x402
//     gate in the HTTP layer, chargedWriteOp for web paths.
//   - Are they going too fast?    CheckPostRate, GuestCallAllowed,
//     videoSearchLimit, SignupRateLimit.
//
// The disagreements were not theoretical. Free tools were paywalled because
// the x402 gate asked the first question with the second one's answer. web_fetch
// was offered to anonymous callers and refused a layer later by its handler.
// wallet_check refused to tell you a price unless you could afford it.
//
// This does not move the enforcement — the gates stay where they are, because
// they run at different moments and one of them has to be in the HTTP layer
// where the payment header is. What it moves is the *decision*, so every gate
// reaches the same conclusion and a test can read the whole policy at once.

import (
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
)

// Policy is what this instance will do about one tool.
type Policy struct {
	Tool string

	// Operation is the wallet operation, empty when the tool has none.
	Operation string

	// Price is what it costs on this instance, in credits. Zero is free, and
	// free is not the same as open — see NeedsAccount.
	Price int

	// NeedsAccount is true when a caller must be somebody, whatever they pay.
	// Money proves funds, not accountability: sending mail from this domain
	// spends its reputation, fetching a URL makes this server act for you, and
	// a per-account ration cannot ration an anonymous caller.
	NeedsAccount bool

	// Payable is true when an anonymous caller can unlock this by paying. A
	// tool that is both priced and account-only is not payable — offering a
	// challenge for it takes money and refuses the call anyway.
	Payable bool
}

// PolicyFor answers for one tool by name. Zero Policy for a name that is not a
// tool.
func PolicyFor(name string) Policy {
	for i := range tools {
		if !toolMatches(tools[i], name) {
			continue
		}
		return policyOf(tools[i])
	}
	return Policy{}
}

func policyOf(t Tool) Policy {
	p := Policy{Tool: t.Name, Operation: t.WalletOp}
	if t.WalletOp != "" {
		// Asked of quota directly. This was a hook set by main.go to the
		// wallet's lookup, back when the price list lived inside the wallet
		// and this package could not import it. It does not any more.
		p.Price = quota.GetOperationCost(t.WalletOp)
	}
	// Three ways a tool comes to need a caller, and the policy has to know all
	// of them or it reports tools as open that are not. Declared on the tool;
	// implied by taking an account id; or declared once on the service, which
	// is how the path-backed ones do it — mail_inbox carries no flag, and
	// mail's Spec is Scoped. This is the same question MCPToolNeedsAuth asks,
	// asked from the same sources, so the audit and the gate cannot drift.
	p.NeedsAccount = t.AccountOnly || t.HandleAuth != nil ||
		service.AccountScoped(serviceOf(t.Name))
	if t.OptionalAuth {
		p.NeedsAccount = false
	}
	p.Payable = p.Price > 0 && !p.NeedsAccount
	return p
}

// Policies returns every tool's policy, for the audit and for tests.
func Policies() []Policy {
	out := make([]Policy, 0, len(tools))
	for i := range tools {
		if tools[i].RESTOnly {
			continue
		}
		out = append(out, policyOf(tools[i]))
	}
	return out
}

// Metered reports whether calling this tool costs the caller anything. The
// question the payment gates should be asking, rather than whether a wallet
// operation exists.
func (p Policy) Metered() bool { return p.Price > 0 }

// Open reports whether an anonymous caller can use this tool as it stands:
// free, and not account-only. These are the front door — the tools an agent
// that found this endpoint mid-task can actually try.
func (p Policy) Open() bool { return p.Price == 0 && !p.NeedsAccount }

// Describe is the one-line summary used by the audit and by test failures.
func (p Policy) Describe() string {
	var b strings.Builder
	b.WriteString(p.Tool)
	switch {
	case p.Open():
		b.WriteString(": open")
	case p.NeedsAccount && p.Price > 0:
		b.WriteString(": account only, and charged")
	case p.NeedsAccount:
		b.WriteString(": account only, free")
	default:
		b.WriteString(": payable")
	}
	if p.Price > 0 {
		b.WriteString(" (")
		b.WriteString(p.Operation)
		b.WriteString(")")
	}
	return b.String()
}
