package auth

import (
	"strings"
	"testing"
)

func TestNativeRedirectAllowlist(t *testing.T) {
	for _, uri := range []string{"mu.client.android://oauth/callback", "mu.client.android.debug://oauth/callback"} {
		if !RegisterableRedirect(uri) {
			t.Fatalf("native callback rejected: %s", uri)
		}
	}
	for _, uri := range []string{"mu.client.android://evil/callback", "mu.client.android://oauth/callback?next=evil", "mu.client.android://oauth/other", "other://oauth/callback"} {
		if RegisterableRedirect(uri) {
			t.Fatalf("unexpected app callback accepted: %s", uri)
		}
	}
}

func TestNativeExchangeRequiresPKCE(t *testing.T) {
	redirect := "mu.client.android://oauth/callback"
	for _, method := range []string{"", "plain"} {
		code := CreateAuthorizationCode("native-test", "unused-account", redirect, "", method, "read", "write", "api:agent")
		if _, err := ExchangeAuthorizationCode(code, "native-test", redirect, ""); err == nil || !strings.Contains(err.Error(), "S256") {
			t.Fatalf("native exchange without S256 accepted: %v", err)
		}
	}
	code := CreateAuthorizationCode("native-test", "unused-account", redirect, strings.Repeat("a", 43), "S256", "read", "write", "api:agent")
	if _, err := ExchangeAuthorizationCode(code, "native-test", redirect, "wrong-verifier"); err == nil {
		t.Fatal("native exchange accepted wrong verifier")
	}
}
