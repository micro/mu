package api

// TokenHeader is the header a caller may use instead of Authorization.
const TokenHeader = "X-Micro-Token"

// This file used to hold Endpoints: twenty REST endpoints written out by hand
// with their params and response shapes, plus Register and Markdown to extend
// and print them.
//
// Nothing read it. The /api page's reference came from the tool registry while
// its playground selector came from here, so the selector offered twenty
// endpoints and the page below documented twenty-five, and neither noticed when
// the other changed. Both then read restTools(), derived from the registered
// tools, so the page could not disagree with itself.
//
// The page is gone too. Documenting REST alongside MCP made a reader choose a
// way in before they could start, and MCP is the one that matters — /api now
// redirects to /mcp. The paths still answer for anything already calling them,
// which is why restTools survives: the surfaces must stay unmixed even with
// nothing rendering them.

// restTools is the set of tools reachable over plain HTTP, keyed by method and
// path.
//
// Several tools share one path with different verbs (GET/POST/PATCH/DELETE on
// /blog/post), so the endpoint identity is method+path, not the tool name.
func restTools() []Tool {
	seen := map[string]bool{}
	var out []Tool
	for _, t := range sortedTools() {
		if t.Path == "" {
			continue
		}
		if t.Method == "" {
			t.Method = "GET"
		}
		key := t.Method + " " + t.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}
