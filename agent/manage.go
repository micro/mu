package agent

import (
	"context"
	"encoding/json"
	"fmt"
	gmagent "go-micro.dev/v6/agent"
	"mu/internal/service"
	"strings"
)

// Management is available only to the account's general agent, never guests or
// scoped specialists. It cannot issue credentials or grant new external access.
func managementTools(owner string, opts QueryOpts) []gmagent.Option {
	if owner == "" || opts.Public || len(opts.Tools) > 0 {
		return nil
	}
	return []gmagent.Option{
		gmagent.WithTool("micro_agents", "List your focused agents and their tool scopes", map[string]any{}, func(_ context.Context, _ map[string]any) (string, error) {
			var items []map[string]any
			for _, a := range Agents(owner) {
				items = append(items, map[string]any{"id": a.ID, "name": a.Name, "services": a.Services})
			}
			b, e := json.Marshal(items)
			return string(b), e
		}),
		gmagent.WithTool("micro_create_agent", "Create a focused agent when the user asks for one. Specify the minimum service scope. Does not create a token or connect external accounts.", map[string]any{"name": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"}, "services": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, func(_ context.Context, in map[string]any) (string, error) { return createFocusedAgent(owner, in) }),
	}
}

func createFocusedAgent(owner string, in map[string]any) (string, error) {
	name, _ := in["name"].(string)
	prompt, _ := in["prompt"].(string)
	raw, ok := in["services"].([]any)
	if !ok || len(raw) == 0 {
		return "", fmt.Errorf("choose the specific services this agent needs")
	}
	if strings.TrimSpace(prompt) == "" || len(prompt) > 8000 {
		return "", fmt.Errorf("provide instructions of at most 8000 characters")
	}
	var scope []string
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("invalid service")
		}
		if _, ok := service.SpecFor(s); !ok {
			return "", fmt.Errorf("unknown service %s", s)
		}
		scope = append(scope, s)
	}
	a, _, err := CreateAgent(owner, name, Hosted, prompt, "", scope, false)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(map[string]any{"id": a.ID, "name": a.Name, "url": chatPath(owner, a.ID), "services": a.Services})
	return string(b), err
}
