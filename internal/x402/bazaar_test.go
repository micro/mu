package x402

import (
	"encoding/json"
	"testing"
)

// The listing is hand-written rather than built by the SDK, so the shape has to
// be pinned here or it drifts from what a facilitator will accept — and the way
// we would find out is silently not being listed.
func TestBazaarListingShape(t *testing.T) {
	ext := BazaarExtension("web_search", "Search the web", map[string]any{
		"type":       "object",
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
		"required":   []string{"query"},
	})
	if ext == nil {
		t.Fatal("no extension built")
	}

	// Round-trip, because what matters is the JSON a facilitator reads.
	b, err := json.Marshal(ext)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	info, ok := got["info"].(map[string]any)
	if !ok {
		t.Fatal("no info object")
	}
	input, ok := info["input"].(map[string]any)
	if !ok {
		t.Fatal("no info.input object")
	}
	if input["type"] != "mcp" {
		t.Errorf(`info.input.type = %v, want "mcp"`, input["type"])
	}
	if input["toolName"] != "web_search" {
		t.Errorf("toolName = %v", input["toolName"])
	}
	if input["transport"] != "streamable-http" {
		t.Errorf("transport = %v, want streamable-http", input["transport"])
	}
	if _, ok := input["inputSchema"].(map[string]any); !ok {
		t.Error("inputSchema is missing or not an object")
	}

	// The schema half describes the info half; a facilitator validates one
	// against the other before indexing.
	schema, ok := got["schema"].(map[string]any)
	if !ok {
		t.Fatal("no schema object")
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v", schema["$schema"])
	}
	props := schema["properties"].(map[string]any)["input"].(map[string]any)
	if props["additionalProperties"] != false {
		t.Error("input schema should forbid additional properties")
	}
	inner := props["properties"].(map[string]any)
	typeProp := inner["type"].(map[string]any)
	if typeProp["const"] != "mcp" {
		t.Errorf(`schema input.type.const = %v, want "mcp"`, typeProp["const"])
	}
	if _, described := inner["description"]; !described {
		t.Error("a described tool must have description in its schema")
	}
}

// A tool with no arguments still needs a schema object, or the listing
// describes an inputSchema that is not there.
func TestBazaarListingWithoutArguments(t *testing.T) {
	ext := BazaarExtension("news_list", "", nil)
	input := ext["info"].(map[string]any)["input"].(map[string]any)
	if _, ok := input["inputSchema"].(map[string]any); !ok {
		t.Error("inputSchema missing for a tool with no arguments")
	}
	// No description means no description property to validate against.
	inner := ext["schema"].(map[string]any)["properties"].(map[string]any)["input"].(map[string]any)["properties"].(map[string]any)
	if _, present := inner["description"]; present {
		t.Error("schema describes a description the info does not carry")
	}
}

// An unnamed tool is not a listing.
func TestBazaarNeedsAToolName(t *testing.T) {
	if ext := BazaarExtension("  ", "something", nil); ext != nil {
		t.Errorf("built a listing with no tool name: %v", ext)
	}
}

// Off unless asked for. Listing tells a third party this instance exists, what
// it sells and what it charges, which a self-hosted instance must opt into.
func TestBazaarIsOffByDefault(t *testing.T) {
	if BazaarEnabled() {
		t.Error("bazaar is on without X402_BAZAAR being set")
	}
	if ext := BazaarExtensions("web_search", "Search", nil); ext != nil {
		t.Errorf("listing published while disabled: %v", ext)
	}
}
