package service

import (
	"context"
	"testing"
)

func TestAccountRoundTrip(t *testing.T) {
	ctx := WithAccount(context.Background(), "alice")
	if got := AccountFrom(ctx); got != "alice" {
		t.Errorf("AccountFrom = %q, want alice", got)
	}
}

func TestAccountFromEmptyContextIsGuest(t *testing.T) {
	if got := AccountFrom(context.Background()); got != "" {
		t.Errorf("AccountFrom = %q, want empty for a guest", got)
	}
}

// A guest request must not inherit the previous caller's identity.
func TestWithAccountEmptyClearsIdentity(t *testing.T) {
	ctx := WithAccount(context.Background(), "alice")
	ctx = WithAccount(ctx, "")
	if got := AccountFrom(ctx); got != "" {
		t.Errorf("AccountFrom = %q after clearing, want empty", got)
	}
}

// ── CallDynamic identity binding ────────────────────────────────

// EchoReq carries an account_id the way real service requests do.
type EchoReq struct {
	AccountID string `json:"account_id"`
	Note      string `json:"note"`
}

type EchoRsp struct {
	SawAccount string `json:"saw_account"`
	SawMeta    string `json:"saw_meta"`
}

type EchoSrv struct{}

func (EchoSrv) Echo(ctx context.Context, req *EchoReq, rsp *EchoRsp) error {
	rsp.SawAccount = req.AccountID
	rsp.SawMeta = AccountFrom(ctx)
	return nil
}

func registerEcho(t *testing.T) {
	t.Helper()
	if err := Register("echoprobe", new(EchoSrv)); err != nil {
		t.Fatalf("register: %v", err)
	}
}

// The whole point of #1443: a caller cannot scope a call to another user by
// naming them in the arguments.
func TestCallDynamicIgnoresForgedAccountID(t *testing.T) {
	registerEcho(t)

	ctx := WithAccount(context.Background(), "alice")
	rsp, err := CallDynamic(ctx, "echoprobe", "echo", map[string]any{
		"account_id": "mallory", // forged
		"note":       "hi",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if rsp["saw_account"] != "alice" {
		t.Errorf("handler saw account %v, want alice — forged id was not overwritten", rsp["saw_account"])
	}
	if rsp["saw_meta"] != "alice" {
		t.Errorf("handler context identity = %v, want alice", rsp["saw_meta"])
	}
}

// A guest caller cannot acquire an identity by supplying one.
func TestCallDynamicGuestCannotClaimAnAccount(t *testing.T) {
	registerEcho(t)

	rsp, err := CallDynamic(context.Background(), "echoprobe", "echo", map[string]any{
		"account_id": "alice",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if rsp["saw_account"] != "" && rsp["saw_account"] != nil {
		t.Errorf("guest call reached the handler as %v, want no account", rsp["saw_account"])
	}
}

// Identity set at the boundary reaches the handler without the caller passing
// anything at all.
func TestCallDynamicStampsIdentityWhenAbsent(t *testing.T) {
	registerEcho(t)

	ctx := WithAccount(context.Background(), "bob")
	rsp, err := CallDynamic(ctx, "echoprobe", "echo", map[string]any{"note": "x"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if rsp["saw_account"] != "bob" {
		t.Errorf("handler saw account %v, want bob", rsp["saw_account"])
	}
}

// Agent tools are derived from the registry, so a service is model-visible by
// default. Anything whose side effects should only follow from a user's own
// action must be excluded — the agent reads attacker-controlled text, so a tool
// it holds is a tool prompt injection holds.
func TestServicesWithUserOnlySideEffectsAreNotAgentTools(t *testing.T) {
	for _, name := range []string{"wallet", "db"} {
		if AgentExposed(name) {
			t.Errorf("%q is exposed to the model; charging credits and deleting records must not be model-driven", name)
		}
	}
	for _, name := range []string{"news", "weather", "web", "search", "mail"} {
		if !AgentExposed(name) {
			t.Errorf("%q should be an agent tool", name)
		}
	}
}
