package test

// Tool descriptions point at other tools — "use news_list for what is happening
// generally", "call index_search instead". That cross-referencing is most of
// what makes a surface of eighty tools navigable, and it is also the part that
// rots silently: renaming or removing a tool leaves every description that
// named it pointing at nothing, and the model reading it has no way to know.
//
// This happened immediately. chat_ask was removed, and agent_ask's description
// went on telling agents to use it for a whole commit.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"mu/internal/tool"

	"mu/internal/api"
)

// A tool name in prose: service_method, the shape every tool here has.
var toolRef = regexp.MustCompile(`\b([a-z][a-z0-9]*_[a-z0-9_]+)\b`)

// Words that look like tool names and are not. Kept short deliberately: the
// cheapest way for this test to go wrong is a growing list of excuses.
var notTools = map[string]bool{
	"service_method": true, // the naming rule, stated in prose
	"content_type":   true,
	"day_start":      true,
	"day_end":        true,
	"new_slug":       true,
	"from_lat":       true,
	"from_lon":       true,
	"to_lat":         true,
	"to_lon":         true,
	"context_id":     true,
	"message_id":     true,
	"in_reply":       true,
}

func TestNoToolDescriptionPointsAtAToolThatIsGone(t *testing.T) {
	registerAll(t)
	tool.DeriveTools()

	real := map[string]bool{}
	for _, tool := range api.Commands() {
		real[tool.Name] = true
		for _, a := range tool.Aliases {
			real[a] = true
		}
	}
	// Hand-registered tools are not in this binary's registry; the source that
	// registers them is the other half of the truth.
	src := []byte(registrationSource(t))
	for _, m := range regexp.MustCompile(`Name:\s*"([a-z][a-z0-9_]*)"`).FindAllStringSubmatch(string(src), -1) {
		real[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`Aliases:\s*\[\]string\{([^}]*)\}`).FindAllStringSubmatch(string(src), -1) {
		for _, a := range regexp.MustCompile(`"([a-z][a-z0-9_]*)"`).FindAllStringSubmatch(m[1], -1) {
			real[a[1]] = true
		}
	}

	// Every description that ships, from both halves of the registry.
	descriptions := map[string]string{}
	for _, tool := range api.Commands() {
		descriptions[tool.Name] = tool.Description
	}
	for _, m := range regexp.MustCompile(`Name:\s*"([a-z][a-z0-9_]*)",\s*(?:Aliases:[^\n]*\n\s*)?Description:\s*"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(string(src), -1) {
		descriptions[m[1]] = m[2]
	}

	var broken []string
	for name, desc := range descriptions {
		for _, m := range toolRef.FindAllStringSubmatch(desc, -1) {
			ref := m[1]
			if notTools[ref] || real[ref] || ref == name {
				continue
			}
			broken = append(broken, name+" points at "+ref)
		}
	}
	sort.Strings(broken)
	if len(broken) > 0 {
		t.Errorf("%d description(s) name a tool that does not exist:\n  %s",
			len(broken), strings.Join(broken, "\n  "))
	}
}
