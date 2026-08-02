package api

import (
	"strings"
	"testing"
)

// REST and MCP are two naming systems over the same services: REST is
// resource-shaped (/news, /mail) and MCP is service_method (news_list,
// mail_inbox). Each list must contain only its own kind.
func TestRESTAndMCPSurfacesDoNotMix(t *testing.T) {
	for _, tool := range mcpTools() {
		if tool.RESTOnly {
			t.Errorf("%q is a REST endpoint but appears in the MCP tool list", tool.Name)
		}
	}
	for _, ep := range restTools() {
		if ep.Path == "" {
			t.Errorf("%q has no HTTP path but appears in the REST endpoint list", ep.Name)
		}
	}
}

// A tool name and its aliases must be unique across the whole surface, or a
// call resolves to whichever was registered first and the other is dead.
func TestToolNamesAndAliasesAreUnique(t *testing.T) {
	seen := map[string]string{}
	claim := func(name, owner string, t *testing.T) {
		if prev, dup := seen[name]; dup {
			t.Errorf("%q is claimed by both %s and %s", name, prev, owner)
			return
		}
		seen[name] = owner
	}
	for _, tool := range sortedTools() {
		claim(tool.Name, tool.Name, t)
		for _, a := range tool.Aliases {
			claim(a, tool.Name+" (alias)", t)
		}
	}
}

// The old names have to keep resolving after the rename to service_method.
//
// Only the statically-registered tools are visible here; the ones main.go
// registers (web_search, index_search, db_create, news_list) carry their legacy
// names through the same Aliases field, checked by TestToolNamesAndAliasesAreUnique
// once the process has registered them.
func TestRenamedToolsKeepTheirOldNames(t *testing.T) {
	for old, want := range map[string]string{
		"mail_read": "mail_inbox",
		"islam":     "islam_today",
		"reminder":  "islam_today",
		"stream":    "stream_list",
	} {
		found := ""
		for _, tool := range sortedTools() {
			if toolMatches(tool, old) {
				found = tool.Name
				break
			}
		}
		if found == "" {
			t.Errorf("legacy name %q no longer resolves to anything", old)
		} else if found != want && !strings.EqualFold(found, want) {
			t.Errorf("legacy name %q resolves to %q, want %q", old, found, want)
		}
	}
}
