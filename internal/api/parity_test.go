package api

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// /tools and /mcp show the same tools.
//
// They are two views of one catalogue: /tools is the browse — grouped by
// service, one clipped line, a price — and /mcp is the reference, with schemas
// and a playground. A reader who counts nine tools on one page and eleven on
// the other has no reason to trust either number, and an agent pointed here
// gets whatever tools/list says regardless.
//
// The four lists on /mcp (nav index, cards, playground selector, and the JSON
// the form builder indexes into) are separately generated, which is how the
// JSON came to carry the two RESTOnly entries the selector did not offer.
func TestToolsPageAndMCPPageShowTheSameTools(t *testing.T) {
	toolsRec := httptest.NewRecorder()
	ToolsPageHandler(toolsRec, httptest.NewRequest("GET", "/tools", nil))
	mcpRec := httptest.NewRecorder()
	MCPHandler(mcpRec, httptest.NewRequest("GET", "/mcp", nil))

	tools := toolsRec.Body.String()
	mcp := mcpRec.Body.String()

	tiles := names(`tool-tile-name">([^<]+)<`, tools)
	if len(tiles) == 0 {
		t.Fatal("no tool tiles on /tools — the extraction is broken, not the parity")
	}

	for _, view := range []struct {
		what string
		got  []string
	}{
		{"card", names(`<div class="card" id="tool-([^"]+)"`, mcp)},
		{"nav index entry", names(`<a href="#tool-([^"]+)">`, mcp)},
		{"playground option", names(`<option value="([^"]+)">`, mcp)},
		{"JSON metadata key", jsonToolNames(t, mcp)},
	} {
		if d := diff(tiles, view.got); d != "" {
			t.Errorf("/mcp %ss do not match the /tools grid: %s", view.what, d)
		}
	}
}

func names(pattern, body string) []string {
	var out []string
	for _, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(body, -1) {
		if m[1] != "" {
			out = append(out, m[1])
		}
	}
	return out
}

// jsonToolNames pulls the keys out of the `var mcpTools={...};` blob the
// playground's JavaScript indexes into.
func jsonToolNames(t *testing.T, body string) []string {
	m := regexp.MustCompile(`(?s)var mcpTools=(\{.*?\});function`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no mcpTools JSON on /mcp")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("mcpTools JSON does not parse: %v", err)
	}
	out := make([]string, 0, len(parsed))
	for k := range parsed {
		out = append(out, k)
	}
	return out
}

func diff(want, got []string) string {
	in := func(s []string) map[string]bool {
		m := map[string]bool{}
		for _, v := range s {
			m[v] = true
		}
		return m
	}
	w, g := in(want), in(got)
	var missing, extra []string
	for n := range w {
		if !g[n] {
			missing = append(missing, n)
		}
	}
	for n := range g {
		if !w[n] {
			extra = append(extra, n)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing "+strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		parts = append(parts, "extra "+strings.Join(extra, ", "))
	}
	return strings.Join(parts, "; ")
}
