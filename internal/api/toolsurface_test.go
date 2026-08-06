package api

// What the tool surface owes an agent.
//
// These are the properties from Anthropic's "Writing effective tools for
// agents" that can be checked without a model: namespacing, descriptions that
// say enough to choose by, list-shaped tools that can be bounded, and no tool
// that can only be used after a round trip to fetch an opaque id.
//
// A lint rather than an eval. It cannot tell you whether the tools are *good* —
// only an eval with a real model does that, and there is one under -tags=eval.
// What it can do is stop the surface drifting, which matters here more than in
// most codebases: tools are derived from Specs, so a new service ships tools
// automatically and nobody is ever asked whether they should exist or what they
// should say.
//
// Thresholds are deliberately loose. The point is to catch a tool that says
// nothing, not to litigate a good one.

import (
	"strings"
	"testing"
)

// shipped is the tool surface as it exists before any test registers a probe.
// Set by TestMain; see the note there for why the live registry will not do.
var shipped []Tool

// minDescription is about one useful sentence. Below this a description is a
// restatement of the tool's own name, which tells a model choosing between
// sixty tools nothing at all.
const minDescription = 40

// Every tool is namespaced by the service it belongs to, so an agent holding
// tools from several servers can tell whose is whose. This is the convention
// the whole Spec derivation exists to guarantee; a hand-registered tool can
// still break it.
func TestEveryToolIsNamespaced(t *testing.T) {
	for _, tool := range shipped {
		if !strings.Contains(tool.Name, "_") {
			t.Errorf("tool %q has no service prefix, so nothing says where it came from", tool.Name)
		}
	}
}

// A description is what an agent reads when choosing. It is loaded into context
// on every turn, and refining it is documented as one of the highest-leverage
// changes available — which only works if there is something there to refine.
func TestEveryToolSaysEnoughToBeChosen(t *testing.T) {
	var thin []string
	for _, tool := range shipped {
		if len(strings.TrimSpace(tool.Description)) < minDescription {
			thin = append(thin, tool.Name+" ("+tool.Description+")")
		}
	}
	if len(thin) > 0 {
		t.Errorf("%d tool(s) describe themselves in under %d characters, which is a restatement of the name:\n  %s",
			len(thin), minDescription, strings.Join(thin, "\n  "))
	}
}

// A tool whose only required argument is an opaque id cannot be called without
// first calling something else to learn the id. That round trip is a turn, a
// model call and a chance to copy the wrong one — and the guidance is explicit
// that identifiers should be resolvable by name where a name exists.
//
// The rule is not "no ids". It is that a tool taking an id must also accept
// something a person would say out loud.
func TestNothingRequiresAnIdAndOnlyAnId(t *testing.T) {
	var blind []string
	for _, tool := range shipped {
		props := schemaProps(tool)
		if len(props) == 0 {
			continue
		}
		if !props["id"] {
			continue
		}
		named := false
		for _, alt := range []string{"name", "title", "query", "slug", "path", "address", "email"} {
			if props[alt] {
				named = true
				break
			}
		}
		if !named && len(props) == 1 {
			blind = append(blind, tool.Name)
		}
	}
	if len(blind) > 0 {
		t.Errorf("%d tool(s) can only be called with an id fetched from another call:\n  %s",
			len(blind), strings.Join(blind, "\n  "))
	}
}

// schemaProps returns a tool's parameter names.
func schemaProps(t Tool) map[string]bool {
	if len(t.Params) == 0 {
		return nil
	}
	out := make(map[string]bool, len(t.Params))
	for _, p := range t.Params {
		out[strings.ToLower(strings.TrimSpace(p.Name))] = true
	}
	return out
}
