package agent

import (
	"context"
	gmai "go-micro.dev/v6/model"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSearchBudgetAcrossParallelCalls(t *testing.T) {
	var calls atomic.Int32
	handler := limitNativeSearches(5)(func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
		calls.Add(1)
		return gmai.ToolResult{ID: call.ID, Content: "ok"}
	})
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "web_search"
			if i%2 == 0 {
				name = "web.Server.Images"
			}
			handler(context.Background(), gmai.ToolCall{Name: name})
		}(i)
	}
	wg.Wait()
	if calls.Load() != 5 {
		t.Fatalf("executed %d searches", calls.Load())
	}
	if r := handler(context.Background(), gmai.ToolCall{Name: "web_search"}); r.Refused != "search_budget_exhausted" {
		t.Fatal("not refused")
	}
	handler(context.Background(), gmai.ToolCall{Name: "web.Server.Fetch"})
	if calls.Load() != 6 {
		t.Fatal("reading evidence incorrectly capped")
	}
	fresh := limitNativeSearches(5)(func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult { return gmai.ToolResult{Content: "ok"} })
	if r := fresh(context.Background(), gmai.ToolCall{Name: "web_search"}); r.Refused != "" {
		t.Fatal("budget leaked between turns")
	}
}
