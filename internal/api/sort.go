package api

import "sort"

// sortedTools returns the registered tools ordered alphabetically by name, for
// stable, predictable display on /mcp, /api and in the MCP tools/list response.
// The underlying registration slice is left untouched — card ordering and
// name-based execution lookups don't depend on it.
func sortedTools() []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sortedEndpoints returns the REST endpoints grouped by path, then method, so a
// service's endpoints list together and in a stable order on /api.
func sortedEndpoints() []*Endpoint {
	out := make([]*Endpoint, len(Endpoints))
	copy(out, Endpoints)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}
