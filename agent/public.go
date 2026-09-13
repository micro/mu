package agent

import (
	"encoding/json"
	"mu/internal/api"
	"mu/internal/thread"
	"strings"
)

// Resolve chooses only a built-in agent or one owned by this account.
func Resolve(account, slug string) (string, error) {
	if slug == "" {
		return DefaultPlatformAgent, nil
	}
	id, ok := agentSlugTarget(account, slug)
	if !ok {
		return "", api.Fail(404, "not_found", "No agent with that name")
	}
	return id, nil
}

func PublicOperations() []api.Operation {
	return []api.Operation{
		{Name: "agent_list", Description: "List the agents you can ask.", Handle: func(account string, raw json.RawMessage) (any, error) {
			out := []map[string]string{{"name": SlugFor(account, DefaultPlatformAgent), "description": "The default assistant"}}
			for _, a := range Agents(account) {

				out = append(out, map[string]string{"name": Slug(a), "description": a.Description})
			}
			return map[string]any{"agents": out}, nil
		}},
		{Name: "agent_ask", Description: "Ask an agent. Pass the returned thread ID to continue the saved conversation.", Writes: true, Params: []api.ToolParam{
			{Name: "prompt", Type: "string", Description: "Your instruction, up to 8000 bytes.", Required: true},
			{Name: "agent", Type: "string", Description: "Agent name; defaults to Micro."},
			{Name: "thread", Type: "string", Description: "An owned conversation ID to continue."},
		}, Handle: func(account string, raw json.RawMessage) (any, error) {
			var req struct {
				Prompt string `json:"prompt"`
				Agent  string `json:"agent"`
				Thread string `json:"thread"`
			}
			if err := api.Decode(raw, &req); err != nil {
				return nil, err
			}
			if strings.TrimSpace(req.Prompt) == "" || len(req.Prompt) > askLimit {
				return nil, api.Fail(400, "invalid_arguments", "Prompt must contain 1–8000 bytes")
			}
			// An omitted agent continues the conversation with its existing agent.
			id := ""
			if req.Thread != "" {
				t := thread.Get(account, req.Thread)
				if t == nil {
					return nil, api.Fail(404, "not_found", "No conversation with that ID")
				}
				id = t.Agent
			}
			if req.Agent != "" || id == "" {
				var err error
				id, err = Resolve(account, req.Agent)
				if err != nil {
					return nil, err
				}
			}
			if reason, ok := affordable(account); !ok {
				return nil, api.Fail(402, "insufficient_credits", reason)
			}
			res, err := Ask(AskRequest{Account: account, Client: thread.WebClient, On: req.Thread, Text: req.Prompt, Agent: id, Trigger: "api"})
			if err != nil {
				return nil, err
			}
			return apiAnswer{Text: res.Text, Thread: res.Thread, Agent: SlugFor(account, id), Flow: res.Flow}, nil
		}},
	}
}
