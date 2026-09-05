package x402

import (
	"net/url"
	"strings"

	"mu/internal/settings"
)

const mcpTransport = "streamable-http"

// BazaarLookup is wired by the server to the MCP registry. It returns the
// canonical tool name, description and input schema for a priced operation.
var BazaarLookup func(op string) (name, description string, inputSchema map[string]any, ok bool)

// BazaarExtensions describes one paid MCP tool for the x402 Bazaar. It is
// emitted only when the resource is on an alternate public host (for example
// m3o.com in front of micro.mu), so self-hosted Mu instances remain
// private-by-default while an explicitly exposed machine surface is findable.
func BazaarExtensions(op, resource string) map[string]any {
	if !alternatePublicResource(resource) || BazaarLookup == nil {
		return nil
	}
	name, description, inputSchema, ok := BazaarLookup(op)
	if !ok || strings.TrimSpace(name) == "" {
		return nil
	}
	if inputSchema == nil {
		inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}

	input := map[string]any{
		"type":        "mcp",
		"toolName":    name,
		"inputSchema": inputSchema,
		"transport":   mcpTransport,
	}
	if d := strings.TrimSpace(description); d != "" {
		input["description"] = d
	}

	inputProps := map[string]any{
		"type":        map[string]any{"type": "string", "const": "mcp"},
		"toolName":    map[string]any{"type": "string"},
		"inputSchema": map[string]any{"type": "object"},
		"transport":   map[string]any{"type": "string", "enum": []string{mcpTransport}},
	}
	if _, ok := input["description"]; ok {
		inputProps["description"] = map[string]any{"type": "string"}
	}

	return map[string]any{
		"bazaar": map[string]any{
			"info": map[string]any{"input": input},
			"schema": map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":                 "object",
						"properties":           inputProps,
						"required":             []string{"type", "toolName", "inputSchema"},
						"additionalProperties": false,
					},
				},
				"required": []string{"input"},
			},
		},
	}
}

func alternatePublicResource(resource string) bool {
	u, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || u.Hostname() == "" {
		return false
	}
	configured := configuredHost()
	return configured != "" && strings.ToLower(u.Hostname()) != configured
}

func configuredHost() string {
	if v := strings.TrimSpace(settings.Get("MU_DOMAIN")); v != "" {
		v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
		if u, err := url.Parse("https://" + v); err == nil {
			return strings.ToLower(u.Hostname())
		}
	}
	if v := strings.TrimSpace(settings.Get("APP_URL")); v != "" {
		if u, err := url.Parse(v); err == nil {
			return strings.ToLower(u.Hostname())
		}
	}
	return ""
}
