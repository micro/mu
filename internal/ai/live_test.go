package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmai "go-micro.dev/v6/model"
)

func TestLiveTokensBeforeCompletionWithTools(t *testing.T) {
	for _, provider := range []string{"openai", "atlascloud", "openrouter"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			release := make(chan struct{})
			defer close(release)
			tokens := make(chan string, 8)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request["stream"] != true {
					t.Error("provider did not request streaming")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if calls.Add(1) == 1 {
					fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":"}}]}}]}`+"\n\n")
					fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"London\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`+"\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n\n")
				fmt.Fprint(w, `data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`+"\n\ndata: [DONE]\n\n")
			}))
			defer srv.Close()
			var ran atomic.Int32
			model := gmai.New(provider, gmai.WithAPIKey("test"), gmai.WithBaseURL(srv.URL), gmai.WithToolHandler(func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
				if call.Name != "lookup" || call.Input["city"] != "London" {
					t.Errorf("bad tool call: %+v", call)
				}
				ran.Add(1)
				return gmai.ToolResult{Content: `{"temperature":20}`}
			}))
			var starts atomic.Int32
			ctx, ok := LiveTokens(ctx, provider, srv.URL, func() { starts.Add(1) }, func(s string) { tokens <- s })
			if !ok {
				t.Fatal("provider unsupported")
			}
			type result struct {
				r *gmai.Response
				e error
			}
			finished := make(chan result, 1)
			go func() {
				r, e := model.Generate(ctx, &gmai.Request{Prompt: "hello", Tools: []gmai.Tool{{Name: "lookup", Description: "lookup weather", Properties: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}}}})
				finished <- result{r, e}
			}()
			select {
			case tok := <-tokens:
				if tok != "Hello" {
					t.Fatalf("first token=%q", tok)
				}
			case res := <-finished:
				t.Fatalf("finished before first token: %+v", res)
			case <-ctx.Done():
				t.Fatal("no live token")
			}
			select {
			case <-finished:
				t.Fatal("completion did not wait for remaining tokens")
			default:
			}
			release <- struct{}{}
			res := <-finished
			if res.e != nil {
				t.Fatal(res.e)
			}
			if res.r.Answer != "Hello world" {
				t.Errorf("reply=%q", res.r.Answer)
			}
			if res.r.Usage.TotalTokens != 13 {
				t.Errorf("usage=%+v", res.r.Usage)
			}
			if ran.Load() != 1 || starts.Load() != 2 {
				t.Errorf("tools=%d starts=%d", ran.Load(), starts.Load())
			}
		})
	}
}

func TestLiveCompletionRejectsBrokenStreams(t *testing.T) {
	for _, body := range []string{
		`data: {broken}` + "\n\n",
		`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n",
		`data: {"error":{"message":"provider failed"}}` + "\n\n",
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"x","function":{"name":"lookup","arguments":"{"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n",
	} {
		if _, err := collectCompletion(strings.NewReader(body), nil); err == nil {
			t.Errorf("accepted broken stream %q", body)
		}
	}
}

type liveRoundTripper func(*http.Request) (*http.Response, error)

func (f liveRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLiveTransportScopeAndProviderErrors(t *testing.T) {
	for _, mode := range []string{"ordinary", "tool", "other_url", "error", "json"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			if mode != "ordinary" {
				ctx, _ = LiveTokens(ctx, "openai", "https://provider.test", nil, func(string) { t.Error("unexpected token") })
			}
			if mode == "tool" {
				ctx = WithoutLiveTokens(ctx)
			}
			url := "https://provider.test/v1/chat/completions"
			if mode == "other_url" {
				url = "https://tool.test/v1/chat/completions"
			}
			req, _ := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(`{"model":"test"}`))
			status := 200
			body := `{"choices":[]}`
			if mode == "error" {
				status = 429
				body = `{"error":"rate limited"}`
			}
			tr := liveTransport{liveRoundTripper(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				streaming := strings.Contains(string(b), `"stream":true`)
				if streaming != (mode == "error" || mode == "json") {
					t.Errorf("unexpected request: %s", b)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"3"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if string(b) != body || resp.StatusCode != status || resp.Header.Get("Retry-After") != "3" {
				t.Fatal("changed provider response")
			}
		})
	}
}

func TestLiveTransportCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, _ = LiveTokens(ctx, "openai", srv.URL, nil, func(string) { cancel() })
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	finished := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		if err == nil || LiveError(ctx) == nil {
			t.Fatal("cancelled stream was treated as successful")
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("cancelled stream did not close")
	}
}

func TestLiveCompletionPreservesUsageAndHidesReasoning(t *testing.T) {
	body := `: keepalive` + "\n\n" + `data: {"choices":[{"delta":{"reasoning_content":"private","content":"answer"},"finish_reason":"stop"}]}` + "\n\n" + `data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}` + "\n\ndata: [DONE]\n\n"
	visible := ""
	result, err := collectCompletion(strings.NewReader(body), func(s string) { visible += s })
	if err != nil {
		t.Fatal(err)
	}
	if visible != "answer" {
		t.Fatalf("visible output %q", visible)
	}
	var decoded struct {
		Usage struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Usage.Total != 22 {
		t.Fatalf("lost usage: %s", result)
	}
}
