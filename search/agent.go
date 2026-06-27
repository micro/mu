package search

import (
	"fmt"
	"strings"
)

// WebSearchText runs a cached web search and returns model-ready results.
// It is the AI-first accessor behind the web_search agent tool.
func WebSearchText(query string, limit int) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "No query provided."
	}
	if limit <= 0 || limit > 10 {
		limit = 6
	}
	results, err := SearchBraveCached(query, limit)
	if err != nil {
		return "Web search is unavailable right now."
	}
	if len(results) == 0 {
		return fmt.Sprintf("No web results for %q.", query)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Web results for %q:\n", query)
	for _, r := range results {
		desc := strings.Join(strings.Fields(r.Description), " ")
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
		fmt.Fprintf(&sb, "%s — %s (%s)\n", strings.TrimSpace(r.Title), desc, r.URL)
	}
	return sb.String()
}
