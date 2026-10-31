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
