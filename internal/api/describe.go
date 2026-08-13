package api

// Describing a tool to something that is not calling it.
//
// tools/list publishes name, description and argument schema to anyone who
// asks. That same description is what a discovery index wants, and until now
// the only way to get it was to be the tools/list handler. A 402 that wants to
// say "this is what you would be buying" had no way to find out.

import "encoding/json"

// ToolSchema is a tool's arguments as JSON Schema, in the shape tools/list
// publishes.
func ToolSchema(t Tool) map[string]any {
	props := map[string]any{}
	var required []string
	for _, p := range t.Params {
		props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// MCPToolCalled returns the tool a tools/call body names.
//
// Matching goes through toolMatches, the same as dispatch, so an alias
// describes the tool it actually reaches rather than nothing.
func MCPToolCalled(body []byte) (Tool, bool) {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Method != "tools/call" {
		return Tool{}, false
	}
	for i := range tools {
		if toolMatches(tools[i], req.Params.Name) {
			return tools[i], true
		}
	}
	return Tool{}, false
}
