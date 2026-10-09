package server

import (
	"mu/internal/api"
	"net/http"
)

func init() {
	http.HandleFunc("GET /mcp", publicMCPHandler)
	http.HandleFunc("GET /tools", api.ServiceToolsPageHandler)
	http.HandleFunc("GET /tools/", api.ToolPageHandler)
}

// Runtime endpoints always serve services, on both the primary and x402 hosts.
func publicRESTHandler(w http.ResponseWriter, r *http.Request) {
	api.RESTHandler(w, api.CredentialRequest(r))
}
func publicMCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	api.MCPHandler(w, api.CredentialRequest(r))
}
func publicReferenceHandler(w http.ResponseWriter, r *http.Request) {
	api.RESTPageHandler(w, r)
}
