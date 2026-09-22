package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-micro.dev/v6/model"
	"go-micro.dev/v6/model/anthropic"
	"go-micro.dev/v6/model/atlascloud"
	"go-micro.dev/v6/model/openai"
)

func TestMeterCountsProviderToolRounds(t *testing.T) {
	for _, provider := range []string{"openai", "atlascloud", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if provider == "anthropic" {
					if calls == 1 {
						fmt.Fprint(w, `{"content":[{"type":"tool_use","id":"one","name":"lookup","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":30}}`)
					} else {
						fmt.Fprint(w, `{"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":200,"output_tokens":40,"cache_read_input_tokens":50}}`)
					}
					return
				}
				if calls == 1 {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"one","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`)
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":200,"completion_tokens":40,"total_tokens":240}}`)
				}
			}))
			defer srv.Close()
			old := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: meterTransport{liveTransport{srv.Client().Transport}}}
			defer func() { http.DefaultClient = old }()
			ctx, meter := MeterCalls(context.Background(), provider, srv.URL, "test-model")
			opts := []model.Option{model.WithBaseURL(srv.URL), model.WithAPIKey("test"), model.WithModel("test-model"), model.WithToolHandler(func(ctx context.Context, call model.ToolCall) model.ToolResult {
				return model.ToolResult{ID: call.ID, Content: `{"ok":true}`}
			})}
			var p model.Model
			switch provider {
			case "openai":
				p = openai.NewProvider(opts...)
			case "atlascloud":
				p = atlascloud.NewProvider(opts...)
			case "anthropic":
				p = anthropic.NewProvider(opts...)
			}
			_, err := p.Generate(ctx, &model.Request{Prompt: "lookup", Tools: []model.Tool{{Name: "lookup", Description: "Look up a value"}}})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || meter.calls != 2 || meter.reported != 2 || meter.input != 300 || meter.output != 50 {
				t.Fatalf("calls=%d meter=%+v", calls, meter)
			}
			if provider == "anthropic" && (meter.cacheRead != 70 || meter.cacheWrite != 30) {
				t.Fatalf("cache usage missing: %+v", meter)
			}
		})
	}
}

type meterTestTransport func(*http.Request) (*http.Response, error)

func (f meterTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMeterLiveResponsesAndToolIsolation(t *testing.T) {
	ctx, meter := MeterCalls(context.Background(), "openai", "https://provider.test", "test-model")
	ctx, _ = LiveTokens(ctx, "openai", "https://provider.test", nil, nil)
	wire := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n"
	transport := meterTransport{liveTransport{meterTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
	})}}
	for _, c := range []context.Context{ctx, WithoutModelTracking(ctx), context.Background()} {
		req, _ := http.NewRequestWithContext(c, "POST", meter.endpoint, strings.NewReader(`{"model":"test-model"}`))
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if c != ctx && string(body) != wire {
			t.Fatal("unmarked traffic changed")
		}
	}
	if meter.calls != 1 || meter.reported != 1 || meter.input != 12 || meter.output != 7 {
		t.Fatalf("meter=%+v", meter)
	}
}

func TestMeterPreservesUsageBeforeFailure(t *testing.T) {
	ctx, meter := MeterCalls(context.Background(), "openai", "https://provider.test", "test-model")
	bodies := []string{`{"usage":{"prompt_tokens":100,"completion_tokens":20}}`, `{"usage":null}`, `{"error":"rate limited"}`}
	for i, body := range bodies {
		status := 200
		if i == 2 {
			status = 429
		}
		transport := meterTransport{meterTestTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		req, _ := http.NewRequestWithContext(ctx, "POST", meter.endpoint, nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body.Close()
		if err != nil || string(got) != body || resp.StatusCode != status {
			t.Fatal("provider response changed")
		}
	}
	if meter.calls != 3 || meter.reported != 1 || meter.input != 100 || meter.output != 20 {
		t.Fatalf("meter=%+v", meter)
	}
}
