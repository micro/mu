package auth

// What a token is allowed to reach.
//
// Tokens have carried a Permissions field since they were introduced, and
// nothing ever read it for a tool call: any token could call any tool its
// account could. That is the right default for a personal access token — it is
// you, in another window — and the wrong one for an agent, which is somebody
// else's program holding your credential.
//
// A scope is stored as "service:<name>" entries in Permissions. A token with no
// such entry is unscoped and reaches everything, so every token issued before
// this behaves exactly as it did. A token with even one reaches only those
// services, and the check is default-deny from there.
//
// Deliberately a whitelist of services rather than of tools. Somebody scoping
// an agent is thinking "it can read my news and send mail", not enumerating
// news_list, news_read and news_search — and a tool added to a service later
// should not silently widen a grant, which is why the check resolves the
// service from the tool name rather than storing tool names.

import (
	"net/http"
	"strings"
)

// ScopePrefix marks a Permissions entry that names a reachable service.
const ScopePrefix = "service:"

// ScopeFor builds the Permissions entries for a set of services.
func ScopeFor(services []string) []string {
	out := make([]string, 0, len(services))
	seen := map[string]bool{}
	for _, s := range services {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, ScopePrefix+s)
	}
	return out
}

// Services returns the services a token is confined to, or nil when it is
// unscoped and may reach everything.
func (t *Token) Services() []string {
	if t == nil {
		return nil
	}
	var out []string
	for _, p := range t.Permissions {
		if strings.HasPrefix(p, ScopePrefix) {
			if name := strings.TrimPrefix(p, ScopePrefix); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// Scoped reports whether this token is confined at all.
func (t *Token) Scoped() bool { return len(t.Services()) > 0 }

// AllowsService reports whether a token may reach a service. An unscoped token
// allows everything; a scoped one allows only what it names.
func (t *Token) AllowsService(name string) bool {
	scope := t.Services()
	if len(scope) == 0 {
		return true
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range scope {
		if s == name {
			return true
		}
	}
	return false
}

// TokenFromRequest returns the personal access token a request authenticated
// with, or nil when it authenticated some other way.
//
// Nil for a cookie session, which is a person in a browser: the scope exists to
// confine a program that was handed a credential, and a person signed into
// their own account is not that. Nil also means "no restriction", so every
// caller must treat a nil token as unscoped rather than as denied.
func TokenFromRequest(r *http.Request) *Token {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("X-Micro-Token"))
	}
	if raw == "" {
		return nil
	}
	if len(raw) > 7 && strings.EqualFold(raw[:7], "bearer ") {
		raw = strings.TrimSpace(raw[7:])
	}
	if raw == "" {
		return nil
	}
	return tokenByRaw(raw)
}

// tokenByRaw finds the stored token matching a presented secret. Same
// comparison ValidatePAT uses, including the padding retry for older tokens.
func tokenByRaw(rawToken string) *Token {
	id, err := ValidatePATToken(rawToken)
	if err != nil {
		return nil
	}
	mutex.Lock()
	defer mutex.Unlock()
	if t, ok := tokens[id]; ok {
		return t
	}
	return nil
}
