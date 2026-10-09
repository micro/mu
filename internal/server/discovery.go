package server

import (
	"fmt"
	"mu/internal/origin"
	"net/http"
	"strings"
)

func init() {
	http.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		base := strings.TrimRight(origin.URL(r), "/")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "# Mu\n\nOpen source agent runtime\n\n- MCP: %s/mcp\n- Tools: %s/tools\n- API catalogue: %s/api/v1/\n", base, base, base)
		fmt.Fprintln(w, "\nAuthenticate with an account API token. Discover service tools with MCP tools/list or GET /api/v1. Send arguments as JSON POST bodies.")
	})
}
