package agent

import (
	"context"
	"reflect"
	"testing"

	gmai "go-micro.dev/v6/ai"
)

func TestLiveToolHooksIncludeRefusedCalls(t *testing.T) {
	var events []string
	call := gmai.ToolCall{ID: "call-1", Name: "news_News_Search", Input: map[string]any{"q": "today"}}
	hooks := StreamHooks{
		ToolStart: func(run ToolRun) { events = append(events, "start:"+run.ID) },
		ToolEnd:   func(run ToolRun) { events = append(events, "end:"+run.ID) },
	}
	wrapped := streamToolReporter(hooks)(func(ctx context.Context, c gmai.ToolCall) gmai.ToolResult {
		events = append(events, "guard")
		if c.Name != call.Name {
			t.Error("changed tool call")
		}
		return gmai.ToolResult{Refused: "approval"}
	})
	if got := wrapped(context.Background(), call); got.Refused != "approval" {
		t.Fatal("lost refusal")
	}
	if want := []string{"start:call-1", "guard", "end:call-1"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v", events)
	}
}
