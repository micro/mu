package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func resetOAuthTestState(t *testing.T) {
	t.Helper()

	oauthMu.Lock()
	oldClients := oauthClients
	oldCodes := oauthCodes
	oauthClients = map[string]*OAuthClient{}
	oauthCodes = map[string]*OAuthCode{}
	oauthMu.Unlock()

	mutex.Lock()
	oldAccounts := accounts
	oldSessions := sessions
	accounts = map[string]*Account{}
	sessions = map[string]*Session{}
	mutex.Unlock()

	t.Cleanup(func() {
		oauthMu.Lock()
		oauthClients = oldClients
		oauthCodes = oldCodes
		oauthMu.Unlock()

		mutex.Lock()
		accounts = oldAccounts
		sessions = oldSessions
		mutex.Unlock()
	})
}

func s256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func TestExchangeAuthorizationCodeRequiresMatchingPKCE(t *testing.T) {
	resetOAuthTestState(t)

	accountID := "oauth-user"
	mutex.Lock()
	accounts[accountID] = &Account{ID: accountID, Name: "OAuth User", Created: time.Now()}
	mutex.Unlock()

	verifier := "correct horse battery staple verifier"
	code := CreateAuthorizationCode("client-1", accountID, "https://app.example/callback", s256Challenge(verifier), "S256")

	if _, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", "wrong verifier"); err == nil {
		t.Fatal("expected invalid code_verifier error")
	}

	if _, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", verifier); err == nil {
		t.Fatal("expected authorization code to be single-use after a failed exchange")
	}
}

func TestExchangeAuthorizationCodeCreatesSessionForPlainPKCE(t *testing.T) {
	resetOAuthTestState(t)

	accountID := "oauth-user"
	mutex.Lock()
	accounts[accountID] = &Account{ID: accountID, Name: "OAuth User", Created: time.Now()}
	mutex.Unlock()

	code := CreateAuthorizationCode("client-1", accountID, "https://app.example/callback", "plain-verifier", "plain")
	token, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", "plain-verifier")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected session token")
	}

	if _, err := ExchangeAuthorizationCode(code, "client-1", "https://app.example/callback", "plain-verifier"); err == nil {
		t.Fatal("expected authorization code to be single-use")
	}
}
