package server

import (
	"mu/internal/auth"
	"mu/x402/billing"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTokenCannotTransferThroughBrowserRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "transfer-permission-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	for _, permissions := range [][]string{{"read"}, {"read", "write"}, {"read", "service:mail"}} {
		_, raw, err := auth.CreateToken(owner, "test", permissions, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/account/transfer", strings.NewReader(`{"to":"another-account","amount":1}`))
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set("Content-Type", "application/json")
		if scopedRequestAllowed(r) {
			t.Fatal("PAT reached browser route")
		}
		w := httptest.NewRecorder()
		billing.BalanceHandler(w, r)
		if w.Code != 403 {
			t.Fatalf("transfer accepted PAT: %d %s", w.Code, w.Body.String())
		}
		r.URL.Path = "/api/v1/mail/inbox"
		if !scopedRequestAllowed(r) {
			t.Fatal("permission-aware API was blocked")
		}
	}
}

func TestMUCPUsesProductTokenBoundary(t *testing.T) {
	r := httptest.NewRequest("POST", "/mucp", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	if !productClientRequest(r) {
		t.Fatal("MUCP bypasses product credential normalization")
	}
}

func TestMUCPRootAcceptsScopedToken(t *testing.T) {
	const owner = "root-protocol-token-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	for _, tc := range []struct {
		permissions []string
		status      int
	}{
		{[]string{"read", "api:agent"}, 200},
		{[]string{"read", "service:news"}, 403},
	} {
		_, raw, err := auth.CreateToken(owner, "root", tc.permissions, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Micro-Protocol", "mucp/1")
		r.Header.Set("Micro-Service", "assistant")
		r.Header.Set("Micro-Endpoint", "Assistant.ListAgents")
		r.Header.Set("Micro-Id", "root-token-12345678")
		if !scopedRequestAllowed(r) {
			t.Fatal("root rejected before permission-aware dispatch")
		}
		w := httptest.NewRecorder()
		IndexHandler(w, r)
		if w.Code != tc.status {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		r.Header.Del("Micro-Protocol")
		if scopedRequestAllowed(r) {
			t.Fatal("token admitted to ordinary root")
		}
	}
}
