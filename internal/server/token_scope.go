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
	if path.Clean(r.URL.Path) != r.URL.Path {
		return false
	}
	return r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/api/v1/")
}
