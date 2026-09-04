package x402

// Bazaar discovery for explicit public tool surfaces.
//
// Mu previously carried Bazaar discovery behind X402_BAZAAR and removed it
// because self-hosted Mu instances should not silently publish their existence
// and catalogue to a third party. That reasoning still holds for the primary
// instance surface. It does not hold for an explicitly configured alternate
// machine-facing surface such as m3o.com: discovery is the purpose of that
// surface.
//
// The metadata itself still comes from tools/list through BazaarLookup, so this
// is not a second catalogue to keep in sync.

import (
	"net/url"
	"strings"

	"mu/internal/settings"
)

const mcpTransport = "streamable-http"

// BazaarLookup is wired by the server to the MCP registry. It returns the
// canonical tool name, description and input schema for a priced operation.
// Keeping the dependency inverted avoids making x402 depend on the API package.
var BazaarLookup func(op string) (name, description string, inputSchema map[string]any, ok bool)

// BazaarExtensions returns the x402 v2 Bazaar extension for a priced MCP tool.
// It is emitted only on an alternate public resource host. The configured Mu
// host remains private-by-default exactly as before; a reverse proxy that opts
// into another public surface is an explicit publication decision.
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
	publicHost := strings.ToLower(u.Hostname())
	configured := configuredHost()
	return configured != "" && publicHost != configured
}

func configuredHost() string {
	if v := strings.TrimSpace(settings.Get("MU_DOMAIN")); v != "" {
		if u, err := url.Parse("https://" + strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")); err == nil {
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
