package ai

// The pinned agent's StreamAsk replays a completed Generate response. This
// request-scoped bridge streams the compatible provider's HTTP response while
// returning the ordinary completion to its existing tool loop. Tool execution,
// retries, memory and accounting remain owned by that loop.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type liveKey struct{}
type liveRequest struct {
	mu       sync.Mutex
	err      error
	endpoint string
	start    func()
	token    func(string)
}

// LiveTokens opts one native run into live OpenAI-compatible completions.
// Unsupported providers retain their existing streaming implementation.
func LiveTokens(ctx context.Context, provider, baseURL string, start func(), token func(string)) (context.Context, bool) {
	switch provider {
	case "atlascloud":
		if baseURL == "" {
			baseURL = "https://api.atlascloud.ai"
		}
	case "openrouter":
		if baseURL == "" {
			baseURL = openRouterBaseURL
		}
	case "openai":
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
	default:
		return ctx, false
	}
	return context.WithValue(ctx, liveKey{}, &liveRequest{endpoint: strings.TrimRight(baseURL, "/") + "/v1/chat/completions", start: start, token: token}), true
}

// LiveError reports a broken stream even if a provider swallowed its follow-up
// error. A successful retry clears it, so normal provider retries still work.
func LiveError(ctx context.Context) error {
	live, _ := ctx.Value(liveKey{}).(*liveRequest)
	if live == nil {
		return nil
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	return live.err
}

// WithoutLiveTokens keeps HTTP calls made by tools outside the provider bridge.
func WithoutLiveTokens(ctx context.Context) context.Context {
	return context.WithValue(ctx, liveKey{}, (*liveRequest)(nil))
}

func init() {
	// These providers use http.DefaultClient and offer no client option. All
	// traffic without the private context marker passes through untouched.
	next := http.DefaultClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	http.DefaultClient.Transport = liveTransport{next}
}

type liveTransport struct{ next http.RoundTripper }

func (t liveTransport) RoundTrip(req *http.Request) (_ *http.Response, roundErr error) {
	live, _ := req.Context().Value(liveKey{}).(*liveRequest)
	if live == nil || req.Method != http.MethodPost || req.URL.String() != live.endpoint || req.Body == nil {
		return t.next.RoundTrip(req)
	}
	defer func() {
		live.mu.Lock()
		live.err = roundErr
		live.mu.Unlock()
	}()
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	setBody := func(b []byte) {
		clone.Body = io.NopCloser(bytes.NewReader(b))
		clone.ContentLength = int64(len(b))
		clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
	}
	// Requests already streaming belong to the provider's own Stream reader.
	if string(payload["stream"]) == "true" {
		setBody(body)
		return t.next.RoundTrip(clone)
	}
	payload["stream"] = json.RawMessage(`true`)
	payload["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	body, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	setBody(body)
	clone.Header.Set("Accept", "text/event-stream")
	resp, err := t.next.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	// Preserve provider status and error bodies (including retry hints).
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		// Some compatible endpoints ignore stream:true and return ordinary JSON.
		return resp, nil
	}
	defer resp.Body.Close()
	if live.start != nil {
		live.start()
	}
	result, err := collectCompletion(resp.Body, live.token)
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(result))
	resp.ContentLength = int64(len(result))
	resp.Header = resp.Header.Clone()
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Content-Encoding")
	resp.TransferEncoding = nil
	return resp, nil
}

type liveTool struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// collectCompletion accepts SSE frames, joining tool argument fragments without
// executing them. Only visible content is emitted; reasoning stays private.
func collectCompletion(r io.Reader, token func(string)) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	var content, refusal, reasoning strings.Builder
	tools := map[int]*liveTool{}
	var usage json.RawMessage
	finish := ""
	done := false
	var data []string
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		raw := strings.Join(data, "\n")
		data = nil
		if raw == "[DONE]" {
			done = true
			return nil
		}
		var chunk struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Index int `json:"index"`
				Delta struct {
					Content   string     `json:"content"`
					Refusal   string     `json:"refusal"`
					Reasoning string     `json:"reasoning_content"`
					Tools     []liveTool `json:"tool_calls"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
			return fmt.Errorf("invalid provider stream: %w", err)
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return fmt.Errorf("provider stream error: %s", chunk.Error)
		}
		if len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
			usage = chunk.Usage
		}
		for _, c := range chunk.Choices {
			if c.Index != 0 {
				continue
			}
			if c.Finish != "" {
				finish = c.Finish
			}
			content.WriteString(c.Delta.Content)
			refusal.WriteString(c.Delta.Refusal)
			reasoning.WriteString(c.Delta.Reasoning)
			for _, part := range c.Delta.Tools {
				if part.Index < 0 || part.Index > 1024 {
					return fmt.Errorf("invalid streamed tool index")
				}
				call := tools[part.Index]
				if call == nil {
					call = &liveTool{}
					tools[part.Index] = call
				}
				call.ID += part.ID
				call.Type += part.Type
				call.Function.Name += part.Function.Name
				call.Function.Arguments += part.Function.Arguments
			}
			if c.Delta.Content != "" && token != nil {
				token(c.Delta.Content)
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consume(); err != nil {
				return nil, err
			}
			if done {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := consume(); err != nil {
		return nil, err
	}
	if !done || finish == "" {
		return nil, fmt.Errorf("provider stream ended before completion")
	}
	calls := make([]map[string]any, 0, len(tools))
	for i := 0; i < len(tools); i++ {
		call := tools[i]
		if call == nil || call.ID == "" || call.Function.Name == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return nil, fmt.Errorf("incomplete streamed tool call")
		}
		calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": call.Function})
	}
	message := map[string]any{"role": "assistant", "content": content.String()}
	if len(calls) > 0 {
		message["tool_calls"] = calls
	}
	if refusal.Len() > 0 {
		message["refusal"] = refusal.String()
	}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	return json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": usage})
}

func (t liveTransport) CloseIdleConnections() {
	if next, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		next.CloseIdleConnections()
	}
}
