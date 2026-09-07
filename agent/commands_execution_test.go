package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"mu/internal/app"
	"mu/internal/service"
)

func TestCommandExecutionLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "fail", "empty", "wait"} {
		t.Run(mode, func(t *testing.T) {
			name := "command" + mode
			if err := service.Register(service.Spec{Name: name, Handler: CommandProbe{}, Endpoints: map[string]service.Endpoint{"Read": {}}}); err != nil {
				t.Fatal(err)
			}
			var events []string
			var step Step
			var output string
			opts := QueryOpts{Stream: StreamHooks{
				ToolStart: func(r ToolRun) {
					events = append(events, "start")
					if r.Name != name+"_read" {
						t.Error(r)
					}
				},
				ToolEnd: func(ToolRun) { events = append(events, "end") },
				Token:   func(s string) { output += s },
			}, OnStep: func(s Step) { step = s; events = append(events, "step") }}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			text, err := executeCommand(ctx, "owner", service.CommandCall{Service: name, Method: "Read", Args: map[string]any{"mode": mode, "account_id": "victim"}}, opts)
			if mode == "success" {
				if err != nil || text != "Ready for owner" || output != text || !step.OK {
					t.Fatalf("%q %q %+v %v", text, output, step, err)
				}
			} else {
				if err == nil || output != "" {
					t.Fatalf("failure leaked output: %q %v", output, err)
				}
				if step.OK {
					t.Fatal("failed call marked successful")
				}
			}
			if strings.Join(events, ",") != "start,end,step" {
				t.Fatalf("events=%v", events)
			}
		})
	}
}

func TestCommandGatewayEnforcesChargesAndRefusals(t *testing.T) {
	old := service.Gate
	t.Cleanup(func() { service.Gate = old })
	for _, refuse := range []bool{false, true} {
		t.Run(fmt.Sprint(refuse), func(t *testing.T) {
			name := "commandpaid"
			if refuse {
				name = "commandrefused"
			}
			allowed, charged := 0, 0
			service.Gate.Allow = func(who, op string) (bool, error) {
				allowed++
				if who != "owner" || op != "command.test" {
					t.Errorf("wrong billing %s %s", who, op)
				}
				if refuse {
					return false, fmt.Errorf("insufficient credits")
				}
				return true, nil
			}
			service.Gate.Charge = func(who, op string) { charged++ }
			if err := service.Register(service.Spec{Name: name, Handler: CommandProbe{}, Endpoints: map[string]service.Endpoint{"Read": {Cost: "command.test"}}}); err != nil {
				t.Fatal(err)
			}
			_, err := executeCommand(context.Background(), "owner", service.CommandCall{Service: name, Method: "Read", Args: map[string]any{}}, QueryOpts{})
			if allowed != 1 {
				t.Errorf("allow checks=%d", allowed)
			}
			if refuse {
				if err == nil || charged != 0 {
					t.Fatalf("refused request charged or succeeded: %d %v", charged, err)
				}
			} else if err != nil || charged != 1 {
				t.Fatalf("paid request: %d %v", charged, err)
			}
		})
	}
}

func TestCommandRenderingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result map[string]any
		want   string
	}{
		{"summary", map[string]any{"summary": "Cloudy", "text": "model prose"}, "Cloudy"},
		{"text", map[string]any{"text": "No results found."}, "No results found."},
		{"empty", map[string]any{}, ""},
		{"empty items", map[string]any{"items": []any{}, "text": "No results."}, "No results."},
		{"bad shapes", map[string]any{"items": []any{nil, 5, "bad"}, "text": "Fallback"}, "Fallback"},
		{"missing fields", map[string]any{"items": []any{map[string]any{"title": "No URL"}}, "text": "Fallback"}, "Fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandText(tc.result); got != tc.want {
				t.Fatalf("%q", got)
			}
		})
	}
	for _, url := range []string{"javascript:alert(1)", "data:text/html,bad", "file:///tmp/private", "//example.com", "https:///missing-host"} {
		text := commandText(map[string]any{"items": []any{map[string]any{"title": "Link", "url": url}}})
		if text != "" {
			t.Errorf("unsafe URL rendered: %s", text)
		}
	}
	text := commandText(map[string]any{"items": []any{map[string]any{"title": "[click](javascript:alert(1)) <script>alert(1)</script>", "url": "https://example.com/"}}})
	rendered := app.RenderString(text)
	if strings.Contains(rendered, "<script>") || strings.Contains(rendered, `href="javascript:`) {
		t.Fatalf("unsafe markup: %s", rendered)
	}
	if !strings.Contains(rendered, "https://example.com/") {
		t.Fatal("lost safe source link")
	}
}

func TestCommandClearsInheritedIdentityForGuests(t *testing.T) {
	if err := service.Register(service.Spec{Name: "commandguest", Handler: CommandProbe{}, Endpoints: map[string]service.Endpoint{"Read": {}}}); err != nil {
		t.Fatal(err)
	}
	ctx := service.WithAccount(context.Background(), "victim")
	text, err := executeCommand(ctx, "", service.CommandCall{Service: "commandguest", Method: "Read", Args: map[string]any{"account_id": "victim"}}, QueryOpts{})
	if err != nil || text != "Ready for " {
		t.Fatalf("guest inherited identity: %q %v", text, err)
	}
}

func TestCommandMissingEndpointReturnsFailure(t *testing.T) {
	for _, call := range []service.CommandCall{{Service: "missing-command-service", Method: "Read"}, {Service: "commandprobe", Method: "Missing"}} {
		var visible string
		_, err := executeCommand(context.Background(), "", call, QueryOpts{Stream: StreamHooks{Token: func(s string) { visible += s }}})
		if err == nil || visible != "" {
			t.Fatalf("missing endpoint succeeded: %+v %q %v", call, visible, err)
		}
	}
}

func TestCommandFailedServiceDoesNotFallThroughToModel(t *testing.T) {
	if err := service.Register(service.Spec{Name: "commandfailure", Handler: CommandProbe{}, Endpoints: map[string]service.Endpoint{"Read": {Commands: []service.Command{{Pattern: "command failure", Defaults: map[string]any{"mode": "fail"}}}}}}); err != nil {
		t.Fatal(err)
	}
	_, err := runNative("", "command failure", QueryOpts{})
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") || strings.Contains(err.Error(), "no AI provider") {
		t.Fatalf("lost direct service error: %v", err)
	}
}
