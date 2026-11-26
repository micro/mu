package server

import (
	"net/http"
	"path"
	"strings"

	"mu/agent"
	"mu/internal/api"
	"mu/internal/auth"
)

// Every PAT uses a permission-aware endpoint. Browser pages cannot preserve
// read/write grants, even when a token has no service restriction.
func scopedRequestAllowed(r *http.Request) bool {
	token := auth.TokenFromRequest(api.CredentialRequest(r))
	if token == nil {
		return true
	}
	if mucpRootRequest(r) {
		return true
	}
	p := strings.TrimSuffix(r.URL.Path, "/")
	if path.Clean(p) != p {
		return false
	}
	return (p == "/session" && r.Method == http.MethodGet) || p == "/.well-known/jmap" || p == "/mail/jmap" || strings.HasPrefix(p, "/mail/jmap/") || productClientRequest(r) || p == "/mcp" || p == "/mcp/assistant" || p == "/api/v1" || strings.HasPrefix(p, "/api/v1/")
}

func mucpRootRequest(r *http.Request) bool {
	return r.URL.Path == "/" && len(r.Header.Values("Micro-Protocol")) > 0
}

func productClientRequest(r *http.Request) bool {
	// Mu Pay always serves JSON. Receipt reads and bodyless Start requests
	// from booking apps can arrive without content negotiation headers.
	if mucpRootRequest(r) || agent.PayRequest(r) || agent.AppRequest(r) {
		return true
	}
	p := r.URL.Path
	if r.Method == "GET" {
		if !strings.Contains(r.Header.Get("Accept"), "application/json") {
			return false
		}
	} else if r.Method != "POST" || (!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") && !strings.Contains(r.Header.Get("Accept"), "application/json")) {
		return false
	}
	if p == "/mucp" || p == "/agent" || p == "/agents" || p == "/inbox" || p == "/work" {
		return true
	}
	if id, ok := api.ResourcePath(p, "/work"); ok && id != "" {
		return true
	}
	if id, ok := api.ResourcePath(p, "/inbox"); ok && id != "" {
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
