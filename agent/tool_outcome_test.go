package agent

import (
	"context"
	"strings"
	"testing"

	gmai "go-micro.dev/v6/model"
)

func TestTaskReportSurvivesAnswerGuard(t *testing.T) {
	r := newNativeToolRecorder()
	r.add("### Apps\nAssistant by Asim\nNo problems found.")
	for _, reply := range []string{"", `{"status":"blocked","summary":"Could not publish"}`, `{"status":"done","summary":"Saved","evidence":["Read back saved source"]}`} {
		got, err := nativeAnswer(reply, r, QueryOpts{RawReply: true})
		if err != nil || got != reply {
			t.Fatalf("outcome replaced: %q, %v", got, err)
		}
	}
	got, _ := nativeAnswer("", r, QueryOpts{})
	if !strings.Contains(got, "Assistant") {
		t.Fatalf("ordinary chat lost its fallback: %q", got)
	}
}

func TestStepReporterInspectsCommandExitStatus(t *testing.T) {
	for _, tc := range []struct {
		name, tool string
		result     gmai.ToolResult
		ok         bool
	}{
		{"exit-zero", "shell_Server_Run", gmai.ToolResult{Content: `{"code":0,"output":"saved"}`}, true},
		{"missing-python", "shell_Server_Run", gmai.ToolResult{Content: `{"code":127,"output":"python3: not found"}`}, false},
		{"value-failure", "shell_Server_Run", gmai.ToolResult{Value: map[string]any{"code": 2, "output": "failed"}}, false},
		{"missing-code", "shell_Server_Run", gmai.ToolResult{Content: `{}`}, false},
		{"service-error", "apps_Server_Edit", gmai.ToolResult{Content: `{"error":"permission denied"}`}, false},
		{"refused", "apps_Server_Edit", gmai.ToolResult{Refused: "approval"}, false},
		{"read", "apps_Server_Read", gmai.ToolResult{Content: `{"name":"Assistant"}`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var step Step
			handler := stepReporter(func(s Step) { step = s })(func(context.Context, gmai.ToolCall) gmai.ToolResult { return tc.result })
			result := handler(context.Background(), gmai.ToolCall{Name: tc.tool})
			if step.OK != tc.ok {
				t.Fatalf("OK=%v, want %v", step.OK, tc.ok)
			}
			if result.Content != tc.result.Content {
				t.Fatal("lost command output")
			}
		})
	}
}
