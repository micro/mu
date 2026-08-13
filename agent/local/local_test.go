package local

import "testing"

// gmai.Tool.Properties is the properties map, not the whole schema — every
// provider wraps it itself. Passing the MCP schema straight through nested it a
// level too deep and Anthropic rejected the request as not being draft 2020-12,
// because `{"type":"object","properties":{"type":"object",...}}` describes a
// property named "type" whose schema is the string "object".
//
// Atlas accepted the same payload, which is how this shipped: one provider
// being lenient is not the payload being right, so the shape is pinned here
// rather than left to whichever provider happens to get tested.
func TestPropertiesAreTheInnerMap(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "what to search for"},
			"limit": map[string]any{"type": "integer"},
		},
		"required": []any{"query"},
	}

	props, required := schemaParts(schema)

	if _, leaked := props["type"]; leaked {
		t.Error(`props has a "type" key — the whole schema was passed through instead of its properties`)
	}
	if _, leaked := props["properties"]; leaked {
		t.Error(`props has a "properties" key — the schema is nested one level too deep`)
	}
	if _, ok := props["query"]; !ok {
		t.Errorf("query is missing from the properties: %v", props)
	}
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("required = %v, want [query]", required)
	}
}

// A tool taking no arguments still has to describe itself as an object with
// none; a nil map would send a null input_schema.
func TestNoArgumentsGivesAnEmptyMap(t *testing.T) {
	for _, schema := range []map[string]any{
		nil,
		{},
		{"type": "object"},
		{"type": "object", "properties": map[string]any{}},
	} {
		props, required := schemaParts(schema)
		if props == nil {
			t.Errorf("props is nil for %v; input_schema would be null", schema)
		}
		if len(required) != 0 {
			t.Errorf("required = %v, want none", required)
		}
	}
}

// The provider never sends "required", so it has to survive in the description
// or the model is guessing which arguments a tool needs.
func TestRequiredArgumentsSurviveInTheDescription(t *testing.T) {
	got := describe("Search the web", []string{"query"})
	if want := "Search the web. Required arguments: query."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if got := describe("Read recent news", nil); got != "Read recent news" {
		t.Errorf("a tool with no required arguments gained text: %q", got)
	}
}

// Malformed schemas must not panic — they come from a remote server we do not
// control, and one bad tool should not take the whole catalogue down.
func TestOddSchemasAreSurvivable(t *testing.T) {
	for _, schema := range []map[string]any{
		{"properties": "not a map"},
		{"required": "not a list"},
		{"required": []any{1, 2, 3}},
		{"properties": map[string]any{"a": "not a schema"}},
	} {
		props, _ := schemaParts(schema)
		if props == nil {
			t.Errorf("props is nil for %v", schema)
		}
	}
}
