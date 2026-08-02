package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestVerifyReadsSession pins the contract Verify depends on: /session answers
// 200 for everyone, and a token that was not accepted comes back as the guest
// session rather than an error status.
func TestVerifyReadsSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session" {
			t.Errorf("Verify hit %s, want /session", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") == "Bearer good" {
			w.Write([]byte(`{"id":"s1","type":"session","account":"alice","created":1}`))
			return
		}
		w.Write([]byte(`{"type":"guest"}`))
	}))
	defer srv.Close()

	if err := NewClient(&ResolvedConfig{URL: srv.URL, Token: "good"}).Verify(); err != nil {
		t.Fatalf("accepted token reported as bad: %v", err)
	}
	if err := NewClient(&ResolvedConfig{URL: srv.URL, Token: "bad"}).Verify(); err == nil {
		t.Fatal("a guest session must not count as logged in")
	}
}
