package x402

import "testing"

func TestBazaarExtensionsOnlyOnAlternatePublicSurface(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")
	old := BazaarLookup
	t.Cleanup(func() { BazaarLookup = old })
	BazaarLookup = func(op string) (string, string, map[string]any, bool) {
		if op != "web_search" {
			t.Fatalf("lookup op = %q", op)
		}
		return "web_search", "Search the web", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		}, true
	}

	if got := BazaarExtensions("web_search", "https://micro.mu/mcp"); got != nil {
		t.Fatalf("primary surface unexpectedly advertised Bazaar metadata: %#v", got)
	}

	ext := BazaarExtensions("web_search", "https://m3o.com/mcp")
	if ext == nil {
		t.Fatal("alternate public surface did not advertise Bazaar metadata")
	}
	bazaar, ok := ext["bazaar"].(map[string]any)
	if !ok {
		t.Fatalf("bazaar extension missing: %#v", ext)
	}
	info := bazaar["info"].(map[string]any)
	input := info["input"].(map[string]any)
	if got := input["type"]; got != "mcp" {
		t.Fatalf("input.type = %#v", got)
	}
	if got := input["toolName"]; got != "web_search" {
		t.Fatalf("toolName = %#v", got)
	}
	if got := input["transport"]; got != "streamable-http" {
		t.Fatalf("transport = %#v", got)
	}
	schema := input["inputSchema"].(map[string]any)
	required := schema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("input schema required = %#v", required)
	}
}
