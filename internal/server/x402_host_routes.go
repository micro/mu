package server

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/api"
	"mu/internal/origin"
	"mu/internal/settings"
)

// GET on the x402 host is documentation for a machine, not another copy of
// Mu's web application. Method-specific patterns are more specific than the
// existing /mcp and /tools registrations, so POST /mcp still reaches the MCP
// server while the primary host keeps its normal human-facing catalogue.
func init() {
	http.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		if !origin.IsX402Host(r) {
			api.MCPHandler(w, r)
			return
		}
		base := strings.TrimRight(origin.URL(r), "/")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%s MCP\n\n", x402HostName())
		fmt.Fprintf(w, "Endpoint: %s/mcp\n", base)
		fmt.Fprintln(w, "Transport: streamable-http")
		fmt.Fprintln(w, "Methods: initialize, tools/list, tools/call")
		fmt.Fprintf(w, "Catalogue: %s/tools\n", base)
		fmt.Fprintln(w, "Payments: HTTP 402/x402 on priced calls")
	})

	http.HandleFunc("GET /tools", func(w http.ResponseWriter, r *http.Request) {
		if !origin.IsX402Host(r) {
			api.ToolsPageHandler(w, r)
			return
		}
		base := strings.TrimRight(origin.URL(r), "/")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%s tools\n\n", x402HostName())
		fmt.Fprintln(w, "The live tool catalogue is available through MCP tools/list.")
		fmt.Fprintf(w, "MCP: %s/mcp\n", base)
		fmt.Fprintf(w, "HTTP API: %s/api/v1/\n", base)
		fmt.Fprintf(w, "Agent metadata: %s/llms.txt\n", base)
		fmt.Fprintln(w, "Priced tools return HTTP 402 with x402 payment requirements.")
	})

	// A human tool-detail page is useful on the primary host but is the wrong
	// representation here. Keep the machine door small and point callers at the
	// canonical schema-bearing catalogue instead.
	http.HandleFunc("GET /tools/", func(w http.ResponseWriter, r *http.Request) {
		if !origin.IsX402Host(r) {
			api.ToolPageHandler(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "Tool pages are not served on this host. Use MCP tools/list for names, descriptions and input schemas.")
	})
}

func x402HostName() string {
	v := strings.TrimSpace(settings.Get("X402_HOST"))
	if v == "" {
		return "Mu"
	}
	v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
	if i := strings.IndexByte(v, '/'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		return strings.ToUpper(parts[len(parts)-2])
	}
	if v != "" {
		return strings.ToUpper(v)
	}
	return "Mu"
}
