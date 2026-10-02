package auth

import (
	"testing"
	"time"
)

func TestListTokensOmitsExpiredOAuth(t *testing.T) {
	now := time.Now()
	fixtures := []*Token{
		{ID: "expired", Account: "owner", OAuthClientID: "chatgpt", ExpiresAt: now.Add(-time.Hour)},
		{ID: "legacy", Account: "owner", Name: "OAuth: old-chatgpt", ExpiresAt: now.Add(-time.Hour)},
		{ID: "active", Account: "owner", OAuthClientID: "chatgpt", ExpiresAt: now.Add(time.Hour)},
		{ID: "legacy-active", Account: "owner", Name: "OAuth: old-chatgpt", ExpiresAt: now.Add(time.Hour)},
		{ID: "no-expiry", Account: "owner", OAuthClientID: "older-client"},
		{ID: "personal", Account: "owner", Name: "My script", ExpiresAt: now.Add(-time.Hour)},
		{ID: "other-account", Account: "other", OAuthClientID: "chatgpt", ExpiresAt: now.Add(time.Hour)},
	}
	mutex.Lock()
	saved := tokens
	tokens = make(map[string]*Token)
	for _, token := range fixtures {
		tokens[token.ID] = token
	}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); tokens = saved; mutex.Unlock() })

	listed := ListTokens("owner")
	want := map[string]bool{"active": true, "legacy-active": true, "no-expiry": true, "personal": true}
	if len(listed) != len(want) {
		t.Fatalf("listed %d credentials, want %d", len(listed), len(want))
	}
	for _, token := range listed {
		if !want[token.ID] {
			t.Errorf("unexpected credential %s", token.ID)
		}
		delete(want, token.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing credentials: %v", want)
	}
	// Listing is not revocation or deletion. Admin audit still sees the grants.
	if got := OAuthConnections(); len(got["chatgpt"]) != 3 || len(got["old-chatgpt"]) != 2 {
		t.Fatalf("listing changed OAuth audit records: %v", got)
	}
}
