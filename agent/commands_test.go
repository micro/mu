package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mu/internal/service"
	"mu/service/news"
	"mu/service/weather"
	"mu/service/web"
)

func TestCommandLinksExcludeToolInstructions(t *testing.T) {
	text := commandText(map[string]any{"text": "instructions for the model", "items": []any{map[string]any{"title": "A story", "url": "https://example.com/story"}}})
	if !strings.Contains(text, "A story") || !strings.Contains(text, "https://example.com/story") || strings.Contains(text, "instructions") {
		t.Fatal(text)
	}
}
func TestCommandsRespectAgentAndAttachment(t *testing.T) {
	for _, opts := range []QueryOpts{{Extra: "Selected article"}} {
		if _, ok := promptCommand("headlines", opts); ok {
			t.Fatal("bypassed explicit context")
		}
	}
}

// A command reaches the same service gateway without configuring any model.
func TestCommandExecutesWithoutModel(t *testing.T) {
	if err := service.Register(service.Spec{Name: "commandprobe", Handler: CommandProbe{}, Endpoints: map[string]service.Endpoint{"Read": {}}}); err != nil {
		t.Fatal(err)
	}
	var visible string
	text, err := executeCommand(context.Background(), "reader", service.CommandCall{Service: "commandprobe", Method: "Read", Args: map[string]any{}}, QueryOpts{Stream: StreamHooks{Token: func(s string) { visible += s }}})
	if err != nil {
		t.Fatal(err)
	}
	if text != "Ready for reader" || visible != text {
		t.Fatalf("text=%q visible=%q", text, visible)
	}
}

type CommandProbe struct{}
type CommandProbeRequest struct {
	Mode string `json:"mode"`
}
type CommandProbeResponse struct {
	Text string `json:"text"`
}

func (CommandProbe) Read(ctx context.Context, req *CommandProbeRequest, rsp *CommandProbeResponse) error {
	switch req.Mode {
	case "fail":
		return fmt.Errorf("provider unavailable")
	case "empty":
		return nil
	case "wait":
		<-ctx.Done()
		return ctx.Err()
	}
	rsp.Text = "Ready for " + service.AccountFrom(ctx)
	return nil
}

func TestCommandCatalogueAndNativeFastPath(t *testing.T) {
	for _, spec := range []service.Spec{news.Spec, weather.Spec, web.Spec} {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct{ input, service, method string }{
		{"headlines", "news", "List"}, {"news", "news", "List"},
		{"weather in London", "weather", "Lookup"}, {"what's the weather today?", "weather", "Lookup"},
		{"is it going to rain?", "weather", "Lookup"},
		{"search the web for Sam Altman", "web", "Search"},
	}
	for _, tc := range cases {
		call, ok := promptCommand(tc.input, QueryOpts{Public: true})
		if !ok || call.Service != tc.service || call.Method != tc.method {
			t.Errorf("%q: %+v %v", tc.input, call, ok)
		}
	}
	for _, input := range []string{"don't show headlines", "compare the weather in London and Paris", "weather in London tomorrow"} {
		if _, ok := promptCommand(input, QueryOpts{Public: true}); ok {
			t.Errorf("overmatched %q", input)
		}
	}
	for _, opts := range []QueryOpts{{Extra: "Selected source"}, {Tools: []string{"weather"}}} {
		if _, ok := promptCommand("headlines", opts); ok {
			t.Errorf("escaped options: %+v", opts)
		}
	}
	for _, spec := range []service.Spec{news.Spec, weather.Spec, web.Spec} {
		for method, ep := range spec.Endpoints {
			for _, command := range ep.Commands {
				input := strings.ReplaceAll(strings.ReplaceAll(command.Pattern, "{place}", "São Paulo"), "{query}", "Sam Altman and OpenAI")
				call, ok := promptCommand(input, QueryOpts{Public: true})
				if !ok || call.Service != spec.Name || call.Method != method {
					t.Errorf("declared command %q failed: %+v %v", input, call, ok)
				}
				for k, v := range command.Defaults {
					if call.Args[k] != v {
						t.Errorf("%q lost default %s: %v", input, k, call.Args)
					}
				}
			}
		}
	}
	text, err := runNative("", "what's the weather today?", QueryOpts{Public: true})
	if err != nil || !strings.Contains(text, "Which town or city") {
		t.Fatalf("fast path failed: %q %v", text, err)
	}
}

func TestExplicitCommandsNeverUseModel(t *testing.T) {
	for _, spec := range []service.Spec{news.Spec, weather.Spec} {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"weather in London", "/weather London", "/weather in London"} {
		call, ok := promptCommand(input, QueryOpts{System: "Selected agent", Tools: []string{"weather"}})
		if !ok || call.Service != "weather" || call.Method != "Lookup" || call.Args["place"] != "London" {
			t.Fatalf("%q: %+v %v", input, call, ok)
		}
	}
	if _, ok := promptCommand("/weather London", QueryOpts{Extra: "attached prose"}); !ok {
		t.Fatal("explicit command blocked by attachment")
	}
	for _, input := range []string{"/unknown-command", "/news"} {
		answer, err := runNative("", input, QueryOpts{System: "Selected agent", Tools: []string{"weather"}})
		if err == nil || !strings.Contains(err.Error(), "unknown or unavailable command") || answer != "" {
			t.Fatalf("%s: %q %v", input, answer, err)
		}
	}
	answer, err := QueryWithOpts("", "/weather", QueryOpts{Public: true, System: "Selected agent", Extra: "attached prose"})
	if err != nil || !strings.Contains(answer, "Which town or city") {
		t.Fatalf("direct query: %q %v", answer, err)
	}
}
