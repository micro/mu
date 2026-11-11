package auth

import (
	"errors"
	"mu/internal/data"
	"time"
)

const refreshLifetime = 90 * 24 * time.Hour

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// The refresh credential belongs to the existing connection: deleting that
// connection or account also revokes renewal. Only its hash is persisted.
func renewableToken(raw string) (*tokenResponse, error) {
	mutex.Lock()
	defer mutex.Unlock()
	t := tokenBy[tokenKey(raw)]
	if t == nil || t.OAuthClientID == "" {
		return nil, errors.New("missing connection")
	}
	next := *t
	refresh := generateRandomString(43)
	next.RefreshHash = tokenKey(refresh)
	next.RefreshExpires = time.Now().Add(refreshLifetime)
	tokens[t.ID] = &next
	if err := data.SaveJSON("tokens.json", tokens); err != nil {
		tokens[t.ID] = t
		return nil, err
	}
	indexToken(&next)
	return &tokenResponse{raw, refresh, "Bearer", 86400}, nil
}

// Rotation is serialized and persisted before returning either secret. It keeps
// the connection ID and approved scopes; renewal cannot request extra access.
func refreshToken(raw, clientID string) (*tokenResponse, error) {
	if raw == "" || clientID == "" {
		return nil, errors.New("missing refresh credential")
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()
	if oauthClients[clientID] == nil {
		return nil, errors.New("client removed")
	}
	mutex.Lock()
	defer mutex.Unlock()
	hash := tokenKey(raw)
	for _, t := range tokens {
		if t.OAuthClientID == clientID && t.RefreshUsed[hash].After(time.Now()) {
			delete(tokens, t.ID)
			if err := data.SaveJSON("tokens.json", tokens); err != nil {
				tokens[t.ID] = t
				return nil, err
			}
			delete(tokenBy, t.Lookup)
			return nil, errors.New("refresh credential reused")
		}
		if t.RefreshHash != hash || t.OAuthClientID != clientID {
			continue
		}
		acc := accounts[t.Account]
		if acc == nil || acc.Banned || !t.RefreshExpires.After(time.Now()) {
			break
		}
		access, refresh := generateRandomString(43), generateRandomString(43)
		next := *t
		next.Lookup, next.Token = tokenKey(access), ""
		next.RefreshUsed = make(map[string]time.Time)
		for key, expiry := range t.RefreshUsed {
			if expiry.After(time.Now()) {
				next.RefreshUsed[key] = expiry
			}
		}
		next.RefreshUsed[hash] = t.RefreshExpires
		next.RefreshHash = tokenKey(refresh)
		next.RefreshExpires = time.Now().Add(refreshLifetime)
		next.ExpiresAt = time.Now().Add(24 * time.Hour)
		tokens[t.ID] = &next
		if err := data.SaveJSON("tokens.json", tokens); err != nil {
			tokens[t.ID] = t
			return nil, err
		}
		delete(tokenBy, t.Lookup)
		indexToken(&next)
		return &tokenResponse{access, refresh, "Bearer", 86400}, nil
	}
	return nil, errors.New("invalid refresh credential")
}
