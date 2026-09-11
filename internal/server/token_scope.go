package server

import (
	"net/http"
	"path"
	"strings"

	"mu/internal/auth"
)

// Service scopes are enforced by the MCP/REST tool dispatcher. Other doors
// authenticate an account, but cannot preserve a restricted caller's grant.
func scopedRequestAllowed(r *http.Request) bool {
	token := auth.TokenFromRequest(r)
	if token == nil || !token.Scoped() {
		return true
	}
	p := strings.TrimSuffix(r.URL.Path, "/")
	if path.Clean(p) != p {
		return false
	}
	return p == "/mcp" || p == "/api/v1" || strings.HasPrefix(p, "/api/v1/")
}
