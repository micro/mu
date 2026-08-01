package service

import (
	"context"
	"strings"
	"testing"

	"go-micro.dev/v6/metadata"
)

// ExtReq/ExtRsp/ExtSrv stand in for a service defined outside this repo: it
// reads identity from go-micro metadata by the published key, importing
// nothing from Mu.
type ExtReq struct{ Note string }
type ExtRsp struct{ Saw string }
type ExtSrv struct{}

func (ExtSrv) Do(ctx context.Context, _ *ExtReq, rsp *ExtRsp) error {
	rsp.Saw, _ = metadata.Get(ctx, AccountKey)
	return nil
}

func TestRegisterAllRegistersEverySrvice(t *testing.T) {
	err := RegisterAll(map[string]any{
		"extone": new(ExtSrv),
		"exttwo": new(ExtSrv),
	})
	if err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	got := map[string]bool{}
	for _, s := range Services() {
		got[s] = true
	}
	for _, want := range []string{"extone", "exttwo"} {
		if !got[want] {
			t.Errorf("%q did not register", want)
		}
	}
}

// A bad entry must not hide the good ones.
func TestRegisterAllReportsEveryFailure(t *testing.T) {
	err := RegisterAll(map[string]any{"nilhandler": nil, "alsonil": nil})
	if err == nil {
		t.Fatal("expected an error for nil handlers")
	}
	for _, want := range []string{"nilhandler", "alsonil"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// An external service reads identity by the published key alone.
func TestExternalServiceReadsIdentityByPublishedKey(t *testing.T) {
	if err := RegisterAll(map[string]any{"extid": new(ExtSrv)}); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := WithAccount(context.Background(), "alice")
	rsp, err := CallDynamic(ctx, "extid", "do", map[string]any{"note": "x"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if rsp["Saw"] != "alice" && rsp["saw"] != "alice" {
		t.Errorf("external handler saw %v / %v, want alice", rsp["Saw"], rsp["saw"])
	}
}

func TestAccountKeyIsStable(t *testing.T) {
	if AccountKey != "Mu-Account" {
		t.Errorf("AccountKey = %q — this is a published contract and must not change", AccountKey)
	}
}
