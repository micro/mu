package api

// The whole policy, read at once.
//
// Metering, gating and rate limiting were decided in six places that did not
// agree, and each disagreement was found by calling the live endpoint rather
// than by reading the code — which is the argument for a test that reads all
// of it in one go.
//
// These are invariants, not a list of prices. A price is an operator's
// decision and env can change it; what must hold is that the answers stay
// consistent with each other.

import (
	"strings"
	"testing"
)

// Nothing is sold that paying cannot buy.
//
// A tool that is both priced and account-only would offer an anonymous caller a
// payment challenge, take the money — settlement happens in the quota check,
// before the handler runs — and then refuse the call because they still have no
// account. That is the shape web_fetch and video_search had: priced at zero,
// floored to one credit by the challenge builder, and gated by their own
// handler.
//
// Priced *and* account-only is allowed, because it is an honest thing to be:
// mail_send costs credits and needs an account, and a funded wallet is
// deliberately not accountable for this sending domain. What is not allowed is
// offering that tool for sale. Both gates are checked, because they are decided
// in different places and only one of them used to ask.
func TestNothingIsSoldThatPayingCannotBuy(t *testing.T) {
	for _, p := range Policies() {
		if p.Payable && p.NeedsAccount {
			t.Errorf("%s is marked payable but needs an account: %s", p.Tool, p.Describe())
		}
	}
}

// The x402 gate in front of /mcp decides this a second time, from the request
// body rather than the policy, and it used to decide it differently: it read
// the tool's price and ignored its account requirement, so mail_send — priced,
// account-only, and the one tool where a funded wallet is explicitly not enough
// — was offered a 402 to a caller who would then be turned away.
func TestTheGateDoesNotOfferWhatItWillRefuse(t *testing.T) {
	checked := 0
	for i := range tools {
		p := policyOf(tools[i])
		if !p.NeedsAccount || p.Price == 0 {
			continue
		}
		checked++
		body := []byte(`{"method":"tools/call","params":{"name":"` + tools[i].Name + `"}}`)
		if op := MCPWalletOp(body); op != "" {
			t.Errorf("the gate offers to charge %s for %s, which needs an account "+
				"that paying does not provide", op, tools[i].Name)
		}
	}
	if checked == 0 {
		t.Skip("no priced account-only tool in this binary")
	}
}

// A price of zero means nothing is charged, and that is the only thing it
// means. Whether a stranger may call it is a separate flag.
func TestFreeAndOpenAreSeparateAnswers(t *testing.T) {
	free := Policy{Tool: "t", Operation: "", Price: 0}
	if !free.Open() || free.Metered() {
		t.Error("a free tool with no account requirement is not reading as open")
	}
	gated := Policy{Tool: "t", Price: 0, NeedsAccount: true}
	if gated.Open() {
		t.Error("free is being read as open, which is how web_fetch was offered to strangers")
	}
	if gated.Metered() {
		t.Error("a free tool reads as metered, so a gate will ask a stranger to pay")
	}
	paid := Policy{Tool: "t", Operation: "web_search", Price: 2}
	if paid.Open() || !paid.Metered() {
		t.Error("a paid tool is reading as open")
	}
}

// Something has to be reachable without an account. If nothing is, the front
// door has closed and nobody will notice from the inside, because everyone
// testing has a token — which is exactly how it stayed shut.
//
// Scope worth knowing: this test binary has only the tools registered at
// package level in internal/api. The ones main.go registers at startup, and
// the Specs that make a service account-scoped, are not linked in here, so the
// list below is a subset and reads more open than the running instance is. The
// invariants above hold either way; the full table is only true at runtime,
// which is why the definitive check is calling the endpoint.
func TestSomethingIsReachableWithoutAnAccount(t *testing.T) {
	var open []string
	for _, p := range Policies() {
		if p.Open() {
			open = append(open, p.Tool)
		}
	}
	if len(open) == 0 {
		t.Fatal("no tool can be called without an account, so an agent that finds " +
			"this endpoint mid-task cannot try anything and never comes back")
	}
	t.Logf("open in this binary (a subset — see the note above): %s",
		strings.Join(open, ", "))
}

// Every priced tool names the operation it charges, and every named operation
// is one the wallet knows. A tool charging an operation with no price is a
// tool charging nothing while claiming to charge.
func TestEveryPricedToolNamesItsOperation(t *testing.T) {
	for _, p := range Policies() {
		if p.Price > 0 && p.Operation == "" {
			t.Errorf("%s has a price with no operation to charge it against", p.Tool)
		}
	}
}

// PolicyFor answers by alias too, since that is how a caller may name a tool.
func TestPolicyAnswersByAlias(t *testing.T) {
	for i := range tools {
		if len(tools[i].Aliases) == 0 {
			continue
		}
		want := PolicyFor(tools[i].Name)
		got := PolicyFor(tools[i].Aliases[0])
		if got.Tool != want.Tool {
			t.Errorf("alias %q resolved to %q, want %q",
				tools[i].Aliases[0], got.Tool, want.Tool)
		}
		break
	}
}
