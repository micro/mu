package agent

import (
	"context"
	"strings"
	"testing"

	gmai "go-micro.dev/v6/ai"
	"mu/internal/service"
	"mu/service/shell"
)

// A tool answers to the name it is advertised under.
//
// There are two names for every tool. go-micro derives one from the RPC
// endpoint — shell_Server_Run, handler type and all — and that is the only one
// that dispatches. Everything a person or a model reads says shell_run,
// because NativeToolName drops the middle part for display: /tools, /mcp, the
// README, the agent builder, and every system prompt written from any of them.
//
// So a model told to call shell_run called shell_run and was told there is no
// such tool. It is not a niche failure — naming a tool in an instruction is
// exactly what the agent builder invites, which means any agent whose prompt
// mentions one has been naming it wrongly, and the symptom is an agent that
// quietly does not use the tool it was given.
func TestAToolAnswersToTheNameWeShow(t *testing.T) {
	// Importing a service does not register it; Load does. Skipping for want of
	// one would have made this test agree with anything.
	shell.Load()
	if len(service.Specs()) == 0 {
		t.Fatal("shell.Load registered nothing, so this test cannot check names")
	}

	names := advertisedToolNames()

	real, ok := names["shell_run"]
	if !ok {
		t.Fatalf("shell_run is not mapped to anything, so the name every page "+
			"prints is not callable:\n%v", names)
	}
	// The dispatching form is service.Handler.Method — what the RPC client
	// splits on a dot and calls.
	if !strings.HasPrefix(real, "shell.") || !strings.HasSuffix(real, ".Run") {
		t.Errorf("shell_run maps to %q, which is not a dispatchable endpoint", real)
	}

	// And the wrapper actually rewrites it, which is the half that matters:
	// a correct map nothing consults changes nothing.
	var saw string
	wrapped := acceptToolNamesWeAdvertise()(func(_ context.Context, call gmai.ToolCall) gmai.ToolResult {
		saw = call.Name
		return gmai.ToolResult{ID: call.ID}
	})
	wrapped(context.Background(), gmai.ToolCall{ID: "1", Name: "shell_run"})
	if saw != real {
		t.Errorf("a call to shell_run reached the handler as %q, want %q", saw, real)
	}

	// A name it does not know is passed through untouched, so go-micro's own
	// resolution still gets its turn and its error message still means what it
	// says.
	wrapped(context.Background(), gmai.ToolCall{ID: "2", Name: "shell_Server_Run"})
	if saw != "shell_Server_Run" {
		t.Errorf("a derived name was rewritten to %q; it should pass through", saw)
	}
}
