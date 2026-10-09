package server

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/api"
	"mu/internal/origin"
)

// Browser documentation shares the runtime catalogue; POST /mcp remains the protocol endpoint.
func init() {
	http.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept")
		if !origin.IsX402Host(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
			publicMCPHandler(w, r)
			return
		}
		base := strings.TrimRight(origin.URL(r), "/")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%s MCP\n\n", x402Name())
		fmt.Fprintf(w, "Endpoint: %s/mcp\n", base)
		fmt.Fprintln(w, "Transport: streamable-http")
		fmt.Fprintln(w, "Methods: initialize, tools/list, tools/call")
		fmt.Fprintf(w, "Catalogue: %s/tools\n", base)
		fmt.Fprintln(w, hostPaymentDescription())
	})

	http.HandleFunc("GET /tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept")
		if !origin.IsX402Host(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
			api.ServiceToolsPageHandler(w, r)
			return
		}
		base := strings.TrimRight(origin.URL(r), "/")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%s tools\n\n", x402Name())
		fmt.Fprintln(w, "The live tool catalogue is available through MCP tools/list.")
		fmt.Fprintf(w, "MCP: %s/mcp\nHTTP API: %s/api/v1/\nAgent metadata: %s/llms.txt\n", base, base, base)
		fmt.Fprintln(w, hostPaymentDescription())
	})
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
