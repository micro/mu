package inbox

import (
	"encoding/json"
	"mu/internal/api"
	"mu/internal/thread"
)

func PublicOperations() []api.Operation {
	id := api.ToolParam{Name: "id", Type: "string", Description: "An owned conversation ID.", Required: true}
	limit := api.ToolParam{Name: "limit", Type: "integer", Description: "Maximum items, 1–100; default 20."}
	ops := []api.Operation{
		{Name: "inbox_list", Description: "List recent conversations across channels. Reading this list does not mark messages read.", Params: []api.ToolParam{limit}, Handle: func(account string, raw json.RawMessage) (any, error) {
			var req struct {
				Limit int `json:"limit"`
			}
			if err := api.Decode(raw, &req); err != nil {
				return nil, err
			}
			n, err := publicLimit(req.Limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"conversations": thread.List(account, n)}, nil
		}},
		{Name: "inbox_read", Description: "Read the latest messages of a conversation without changing its unread state. Use agent_ask with its thread ID to ask about it.", Params: []api.ToolParam{id, limit}, Handle: func(account string, raw json.RawMessage) (any, error) {
			var req struct {
				ID    string `json:"id"`
				Limit int    `json:"limit"`
			}
			if err := api.Decode(raw, &req); err != nil {
				return nil, err
			}
			n, err := publicLimit(req.Limit)
			if err != nil {
				return nil, err
			}
			t := thread.Get(account, req.ID)
			if t == nil {
				return nil, api.Fail(404, "not_found", "No conversation with that ID")
			}
			return map[string]any{"conversation": t, "messages": thread.Messages(account, req.ID, n)}, nil
		}},
	}
	for _, seen := range []bool{true, false} {
		seen := seen
		name := "inbox_mark_unread"
		description := "Mark a conversation unread."
		if seen {
			name = "inbox_mark_read"
			description = "Mark a conversation read."
		}
		ops = append(ops, api.Operation{Name: name, Description: description, Writes: true, Params: []api.ToolParam{id}, Handle: func(account string, raw json.RawMessage) (any, error) {
			var req struct {
				ID string `json:"id"`
			}
			if err := api.Decode(raw, &req); err != nil {
				return nil, err
			}
			if thread.Get(account, req.ID) == nil {
				return nil, api.Fail(404, "not_found", "No conversation with that ID")
			}
			if seen {
				thread.MarkSeen(account, req.ID)
			} else {
				thread.MarkUnread(account, req.ID)
			}
			return map[string]any{"id": req.ID, "unread": !seen}, nil
		}})
	}
	return ops
}
func publicLimit(n int) (int, error) {
	if n == 0 {
		n = 20
	}
	if n < 1 || n > 100 {
		return 0, api.Fail(400, "invalid_arguments", "Limit must be between 1 and 100")
	}
	return n, nil
}
