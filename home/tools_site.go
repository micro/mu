package home

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/origin"
	"mu/internal/settings"
)

const toolsDescription = "Tools for agents"

func IsX402Host(r *http.Request) bool { return origin.IsX402Host(r) }

// X402Index is the machine-first front page for the optional x402 hostname.
// It has no Mu application shell: the hostname exists for machine discovery,
// pay-per-call access and protocol endpoints.
func X402Index(w http.ResponseWriter, r *http.Request) {
	name := x402Name()
	base := strings.TrimRight(origin.URL(r), "/")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "%s\n%s\n\n", name, toolsDescription)
	fmt.Fprintf(w, "MCP: %s/mcp\n", base)
	fmt.Fprintf(w, "API: %s/api/v1/\n", base)
	fmt.Fprintf(w, "LLMs: %s/llms.txt\n", base)
	fmt.Fprintln(w, "\nPriced calls use HTTP 402/x402. Discover the live catalogue with MCP tools/list.")
}

func x402Name() string {
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

func init() {
	http.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		base := strings.TrimRight(origin.URL(r), "/")
		name := "Mu"
		description := toolsDescription
		if IsX402Host(r) {
			name = x402Name()
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "# %s\n\n%s\n\n", name, description)
		fmt.Fprintf(w, "- MCP: %s/mcp\n", base)
		fmt.Fprintf(w, "- API catalogue: %s/api/v1/\n", base)
		fmt.Fprintf(w, "\nPriced calls use HTTP 402/x402. Discover the current tool names, descriptions and schemas with MCP tools/list.\n")
	})
}
