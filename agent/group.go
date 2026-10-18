package agent

import (
	"context"
	gmai "go-micro.dev/v6/model"
	"mu/internal/group"
	"strings"
)

// GroupTools is the shared workspace boundary. Credentials, personal mail and
// account management are deliberately outside it, even for custom agents.
func GroupTools() []string {
	return []string{"events", "notes", "news", "markets", "weather", "web", "places", "maps", "prayer", "islam"}
}

// Every tool rechecks membership, including after a long-running model call.
func groupToolGuard(owner, actor string) gmai.ToolWrapper {
	return func(next gmai.ToolHandler) gmai.ToolHandler {
		return func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
			if strings.HasPrefix(owner, "group:") {
				g, ok := group.FromScope(owner)
				if !ok || (actor != "" && !group.AllowedAI(g, actor)) {
					return gmai.ToolResult{ID: call.ID, Refused: "not_permitted", Content: `{"error":"Group access was revoked"}`}
				}
			}
			if g, ok := group.FromScope(owner); ok && actor != "" && !group.Admin(g.ID, actor) {
				name := strings.ToLower(strings.ReplaceAll(call.Name, ".", "_"))
				if strings.HasPrefix(name, "events_") && !(strings.HasSuffix(name, "_list") || strings.HasSuffix(name, "_read") || strings.HasSuffix(name, "_free")) {
					return gmai.ToolResult{ID: call.ID, Refused: "not_permitted", Content: `{"error":"Only group admins can change shared schedules"}`}
				}
			}
			return next(ctx, call)
		}
	}
}
