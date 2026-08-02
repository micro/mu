package service

import (
	"context"
	"encoding/json"
	"testing"

	"go-micro.dev/v6/ai"
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
	if err := Register(Spec{Name: "echoprobe", Handler: new(EchoSrv)}); err != nil {
		t.Fatalf("register: %v", err)
	}
}

// The whole point of #1443: a caller cannot scope a call to another user by
// naming them in the arguments. The forged value is dropped rather than
// corrected, so it never reaches the handler at all.
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
	if rsp["saw_meta"] != "alice" {
		t.Errorf("handler context identity = %v, want alice", rsp["saw_meta"])
	}
	if rsp["saw_account"] != "" && rsp["saw_account"] != nil {
		t.Errorf("forged account_id reached the handler as %v; it must be stripped", rsp["saw_account"])
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
	if rsp["saw_meta"] != "" && rsp["saw_meta"] != nil {
		t.Errorf("guest call reached the handler as %v, want no account", rsp["saw_meta"])
	}
	if rsp["saw_account"] != "" && rsp["saw_account"] != nil {
		t.Errorf("supplied account_id reached the handler as %v; it must be stripped", rsp["saw_account"])
	}
}

// Identity set at the boundary reaches the handler without the caller passing
// anything at all.
func TestCallDynamicCarriesIdentityWithoutArguments(t *testing.T) {
	registerEcho(t)

	ctx := WithAccount(context.Background(), "bob")
	rsp, err := CallDynamic(ctx, "echoprobe", "echo", map[string]any{"note": "x"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if rsp["saw_meta"] != "bob" {
		t.Errorf("handler saw account %v, want bob", rsp["saw_meta"])
	}
}

// ── the agent's dispatch path ───────────────────────────────────

// TestIdentitySurvivesAgentToolDispatch closes the gap that kept account_id in
// the request structs. The agent does not call services through CallDynamic —
// go-micro's tool handler dispatches straight to the service — so identity is
// only safe to remove from the arguments if it survives that path too. It has
// to be proved, not assumed: if metadata were dropped here, every
// account-scoped handler would silently see a guest.
func TestIdentitySurvivesAgentToolDispatch(t *testing.T) {
	registerEcho(t)

	handler := ai.NewTools(Registry(), ai.ToolClient(Client())).Handler()
	ctx := WithAccount(context.Background(), "acct-me")

	res := handler(ctx, ai.ToolCall{
		Name:  "echoprobe.EchoSrv.Echo",
		Input: map[string]any{"note": "hello"},
	})

	var got struct {
		SawAccount string `json:"saw_account"`
		SawMeta    string `json:"saw_meta"`
	}
	if err := json.Unmarshal([]byte(res.Content), &got); err != nil {
		t.Fatalf("tool result %q: %v", res.Content, err)
	}
	if got.SawMeta != "acct-me" {
		t.Fatalf("handler saw account %q over the agent's dispatch, want %q", got.SawMeta, "acct-me")
	}
	if got.SawAccount != "" {
		t.Fatalf("request field carried %q; identity must come only from context", got.SawAccount)
	}
}
