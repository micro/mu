package x402

// Being findable.
//
// A priced endpoint nobody can discover is a shop with the lights off. The
// x402 Bazaar is the ecosystem's answer: facilitators index resources by
// reading a discovery extension out of the 402 challenge itself. There is no
// registration and no submission — you are listed because you told a
// facilitator what you were selling at the moment it asked what you charge.
//
// Which suits an MCP server exactly. Every field the listing wants — tool name,
// description, argument schema — is what tools/list already publishes, so a
// listing is a restatement of something true rather than a second catalogue to
// keep in step.
//
// Written out by hand rather than by importing the SDK. The shape is a JSON
// document, the SDK that builds it pulls go-ethereum, and this repository keeps
// its dependencies few enough that the whole EVM stack is in evm.go for the
// same reason. See docs/INSTALL.md for X402_BAZAAR.

import (
	"strings"

	"mu/internal/settings"
)

// bazaarKey is the extension's name in the extensions map, matching
// x402.NewFacilitatorExtension("bazaar").
const bazaarKey = "bazaar"

// mcpTransport is how a caller reaches our tools: one POST of JSON-RPC to
// /mcp, which is what the spec calls streamable-http.
const mcpTransport = "streamable-http"

// BazaarEnabled reports whether this instance advertises itself for indexing.
//
// Off by default, and deliberately: listing announces to a third party that
// this instance exists, what it sells and what it charges. That is the right
// choice for micro.mu and nobody else's to make by default — a self-hosted
// instance behind a company firewall should not start publishing its tool
// catalogue because it upgraded.
//
// Read through settings, not os.Getenv, so /admin/env can set it and it takes
// effect without a restart. It was
// os.Getenv first, which made it the one payment setting an operator could not
// reach from the page where every other payment setting lives, and turning it
// on would have meant editing the environment and restarting.
func BazaarEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(settings.Get("X402_BAZAAR")), "true")
}

// BazaarExtension describes one MCP tool for the discovery index.
//
// The returned map goes under "extensions" in the 402 body. Info carries the
// values; Schema is JSON Schema describing Info, which is the extension's own
// convention — a facilitator validates one against the other before indexing,
// so a listing that does not describe itself correctly is dropped rather than
// stored wrong.
//
// Returns nil when there is no tool name, because a listing whose subject is
// unnamed is not a listing.
func BazaarExtension(tool, description string, inputSchema map[string]any) map[string]any {
	if strings.TrimSpace(tool) == "" {
		return nil
	}
	if inputSchema == nil {
		inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}

	input := map[string]any{
		"type":        "mcp",
		"toolName":    tool,
		"inputSchema": inputSchema,
		"transport":   mcpTransport,
	}
	if d := strings.TrimSpace(description); d != "" {
		input["description"] = d
	}

	// The properties of `input`, as JSON Schema. Mirrors what the SDK's
	// DeclareMcpDiscoveryExtension emits, including the const on type and the
	// enum on transport — a facilitator checks these, so guessing at them
	// would mean a listing that validates nowhere.
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
	}
}
