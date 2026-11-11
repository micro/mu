package auth

import (
	"encoding/json"
	"mu/internal/data"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func refreshFixture(t *testing.T) (*Token, *tokenResponse) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	owner := "refresh-owner"
	SetAccountForTest(&Account{ID: owner, Approved: true})
	t.Cleanup(func() { RemoveAccountForTest(owner) })
	client := RegisterOAuthClient(owner, "test", []string{"https://client.example/"})
	token, raw, err := createToken(owner, "test", []string{"read", "api:agent"}, time.Now().Add(time.Hour), client.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := renewableToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DeleteToken(token.ID, owner); ForceDeleteOAuthClient(client.ClientID) })
	return token, reply
}

func TestRefreshAfterExpiryAndRestart(t *testing.T) {
	token, initial := refreshFixture(t)
	mutex.Lock()
	tokens[token.ID].ExpiresAt = time.Now().Add(-time.Hour)
	if err := data.SaveJSON("tokens.json", tokens); err != nil {
		t.Fatal(err)
	}
	b, err := data.LoadFile("tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var restored map[string]*Token
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	tokens[token.ID] = restored[token.ID]
	indexToken(tokens[token.ID])
	mutex.Unlock()
	if _, err := ValidatePAT(initial.AccessToken); err == nil {
		t.Fatal("expired access accepted")
	}
	if _, err := refreshToken(initial.RefreshToken, "wrong-client"); err == nil {
		t.Fatal("cross-client renewal")
	}
	reply, err := refreshToken(initial.RefreshToken, token.OAuthClientID)
	if err != nil {
		t.Fatal(err)
	}
	if reply.AccessToken == initial.AccessToken || reply.RefreshToken == initial.RefreshToken {
		t.Fatal("credentials not rotated")
	}
	if owner, err := ValidatePAT(reply.AccessToken); err != nil || owner != token.Account {
		t.Fatalf("new access: %s %v", owner, err)
	}
	current, _ := TokenByID(token.ID)
	if strings.Join(current.Permissions, " ") != "read api:agent" {
		t.Fatal("scope changed")
	}
	if _, err := ValidatePAT(initial.RefreshToken); err == nil {
		t.Fatal("refresh accepted as access")
	}
	if _, err := refreshToken(initial.RefreshToken, token.OAuthClientID); err == nil {
		t.Fatal("replay accepted")
	}
	if _, err := refreshToken(reply.RefreshToken, token.OAuthClientID); err == nil {
		t.Fatal("replayed connection not revoked")
	}
	if _, err := ValidatePAT(reply.AccessToken); err == nil {
		t.Fatal("replayed access still valid")
	}
}

func TestRefreshRevocationAndInactivity(t *testing.T) {
	for _, action := range []string{"delete", "ban", "client", "expire"} {
		t.Run(action, func(t *testing.T) {
			token, reply := refreshFixture(t)
			switch action {
			case "delete":
				if err := DeleteToken(token.ID, token.Account); err != nil {
					t.Fatal(err)
				}
			case "client":
				ForceDeleteOAuthClient(token.OAuthClientID)
			case "ban":
				mutex.Lock()
				accounts[token.Account].Banned = true
				mutex.Unlock()
			case "expire":
				mutex.Lock()
				tokens[token.ID].RefreshExpires = time.Now().Add(-time.Second)
				mutex.Unlock()
			}
			if _, err := refreshToken(reply.RefreshToken, token.OAuthClientID); err == nil {
				t.Fatal("revoked grant renewed")
			}
		})
	}
}

func TestRefreshHTTPUsesBodyAndNoStore(t *testing.T) {
	token, reply := refreshFixture(t)
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {reply.RefreshToken}, "client_id": {token.OAuthClientID}, "scope": {"admin"}}
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	OAuthTokenHandler(w, req)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("refresh failed: %d", w.Code)
	}
	current, _ := TokenByID(token.ID)
	if current.HasPermission("admin") {
		t.Fatal("refresh escalated permissions")
	}
	req = httptest.NewRequest("POST", "/oauth/token?"+form.Encode(), nil)
	w = httptest.NewRecorder()
	OAuthTokenHandler(w, req)
	if w.Code == 200 {
		t.Fatal("credentials accepted from URL")
	}
}
