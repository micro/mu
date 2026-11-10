package server

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/origin"
)

// clientCORS exposes only the public-client OAuth exchange and the MUCP binding.
// It never permits credentialed browser requests: clients supply scoped bearer
// tokens obtained through explicit OAuth consent and PKCE, not ambient cookies.
// A successful preflight grants no account or service access.
func clientCORS(w http.ResponseWriter, r *http.Request) bool {
	oauth := r.URL.Path == "/oauth/register" || r.URL.Path == "/oauth/token"
	protocol := mucpRootRequest(r)
	if r.URL.Path == "/" && r.Method == http.MethodOptions {
		for _, h := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
			if strings.EqualFold(strings.TrimSpace(h), "Micro-Protocol") {
				protocol = true
			}
		}
	}
	if !oauth && !protocol {
		return false
	}
	value := r.Header.Get("Origin")
	if value == "" || origin.SameOrigin(r) {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		http.Error(w, "Invalid client origin", http.StatusForbidden)
		return true
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Del("Access-Control-Allow-Credentials")
	w.Header().Set("Access-Control-Expose-Headers", "Micro-Protocol, Micro-Service, Micro-Endpoint, Micro-Id, Micro-Error, Micro-Thread")
	if r.Method == http.MethodOptions {
		method := r.Header.Get("Access-Control-Request-Method")
		if method != http.MethodPost && !(protocol && method == http.MethodGet) {
			http.Error(w, "Method not allowed", 405)
			return true
		}
		allowed := map[string]bool{"content-type": true, "authorization": true, "micro-protocol": true, "micro-service": true, "micro-endpoint": true, "micro-id": true}
		for _, h := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" && !allowed[h] {
				http.Error(w, "Header not allowed", 403)
				return true
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Micro-Protocol, Micro-Service, Micro-Endpoint, Micro-Id")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	if protocol && r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, "Cross-origin clients require a bearer token", http.StatusUnauthorized)
		return true
	}
	// OAuth registration/token exchange are public-client operations. No handler
	// at these two endpoints should ever infer authority from a browser session.
	r.Header.Del("Cookie")
	return false
}
