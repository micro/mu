package server

import (
	"net/http"
	"path"
	"strings"

	"mu/internal/api"
	"mu/internal/auth"
)

// Service scopes are enforced by the MCP/REST tool dispatcher. Other doors
// authenticate an account, but cannot preserve a restricted caller's grant.
func scopedRequestAllowed(r *http.Request) bool {
	token := auth.TokenFromRequest(api.CredentialRequest(r))
	if token == nil || !token.Scoped() {
		return true
	}
	p := strings.TrimSuffix(r.URL.Path, "/")
	if path.Clean(p) != p {
		return false
	}
	return productClientRequest(r) || p == "/mcp" || p == "/api/v1" || strings.HasPrefix(p, "/api/v1/")
}

// Only content-negotiated resource handlers may receive product-scoped credentials.
func productClientRequest(r *http.Request) bool {
	p := r.URL.Path
	if r.Method == "GET" {
		if !strings.Contains(r.Header.Get("Accept"), "application/json") {
			return false
		}
	} else if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return false
	}
	if p == "/agent" || p == "/inbox" || p == "/work" {
		return true
	}
	if !strings.HasPrefix(p, "/agent/") {
		return false
	}
	name := strings.TrimPrefix(p, "/agent/")
	if name == "" || strings.Contains(name, "/") {
		return false
	}
	switch name {
	case "handoff", "agents", "new", "run", "pending", "connect", "api", "mcp":
		return false
	}
	return true
}
