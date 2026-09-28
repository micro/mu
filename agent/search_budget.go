package agent

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	gmai "go-micro.dev/v6/model"
	"mu/internal/settings"
)

func searchLimit() int {
	n, err := strconv.Atoi(settings.Get("AGENT_SEARCH_LIMIT"))
	if err != nil || n < 1 || n > 20 {
		return 5
	}
	return n
}

// One budget per native run, shared by every step and parallel tool call.
func limitNativeSearches(limit int) gmai.ToolWrapper {
	var mu sync.Mutex
	used := 0
	return func(next gmai.ToolHandler) gmai.ToolHandler {
		return func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
			name := NativeToolName(call.Name)
			if name != "web_search" && name != "web_images" {
				return next(ctx, call)
			}
			mu.Lock()
			allowed := used < limit
			if allowed {
				used++
			}
			mu.Unlock()
			if !allowed {
				return gmai.ToolResult{ID: call.ID, Refused: "search_budget_exhausted", Content: fmt.Sprintf(`{"error":"Search budget exhausted: at most %d web/image searches per turn. Use existing results and state any evidence gaps. Do not retry search."}`, limit)}
			}
			return next(ctx, call)
		}
	}
}
