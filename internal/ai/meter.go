package ai

// Provider adapters may make several HTTP calls inside one Generate call and
// return only the first call's usage. Count the responses at the existing HTTP
// bridge instead. This measures provider spend; it never debits an account.
import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/usage"
)

type meterKey struct{}

// CallMeter collects one native run's provider calls, including tool follow-ups
// and completed attempts before a failure. It contains no prompts or answers.
type CallMeter struct {
	mu                                                    sync.Mutex
	endpoint, model                                       string
	anthropic                                             bool
	calls, reported, input, output, cacheRead, cacheWrite int
	latency                                               int64
}

func completionEndpoint(provider, baseURL string) string {
	if baseURL == "" {
		switch provider {
		case "atlascloud":
			baseURL = "https://api.atlascloud.ai"
		case "openrouter":
			baseURL = openRouterBaseURL
		case "openai":
			baseURL = "https://api.openai.com"
		case "anthropic":
			baseURL = "https://api.anthropic.com"
		default:
			return ""
		}
	}
	switch provider {
	case "anthropic":
		return strings.TrimRight(baseURL, "/") + "/v1/messages"
	case "atlascloud", "openrouter", "openai":
		return strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	default:
		return ""
	}
}

// MeterCalls enables accounting only for the configured model endpoint. Other
// providers retain timeline accounting until their wire format is supported.
func MeterCalls(ctx context.Context, provider, baseURL, model string) (context.Context, *CallMeter) {
	endpoint := completionEndpoint(provider, baseURL)
	if endpoint == "" {
		return ctx, nil
	}
	m := &CallMeter{endpoint: endpoint, model: model, anthropic: provider == "anthropic"}
	return context.WithValue(ctx, meterKey{}, m), m
}

// Record writes one aggregate, preserving usage collected before a failed run.
func (m *CallMeter) Record(caller, account, runID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == 0 {
		return
	}
	usage.RecordModels(m.calls)
	app.RecordUsage(providerName(m.model), caller,
		estimateCostCents(m.model, m.input, m.output, m.cacheRead, m.cacheWrite), map[string]any{
			"model": m.model, "account": account, "run_id": runID,
			"input_tokens": m.input, "output_tokens": m.output,
			"cache_read_tokens": m.cacheRead, "cache_creation_tokens": m.cacheWrite,
			"model_calls": m.calls, "reported_calls": m.reported,
			"missing_usage_calls": m.calls - m.reported, "model_duration_ms": m.latency,
		})
	if m.reported != m.calls {
		app.Log("agent", "incomplete provider usage for model %q: %d of %d calls reported tokens", m.model, m.reported, m.calls)
	}
}

type meterTransport struct{ next http.RoundTripper }

func (t meterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m, _ := req.Context().Value(meterKey{}).(*CallMeter)
	if m == nil || req.Method != http.MethodPost || req.URL.String() != m.endpoint {
		return t.next.RoundTrip(req)
	}
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		m.add(nil, false, time.Since(start))
		return resp, err
	}
	// The live bridge returns JSON after collecting SSE. Preserve other bodies
	// byte-for-byte and let their readers consume them normally.
	resp.Body = &meterBody{ReadCloser: resp.Body, meter: m, start: start,
		json: resp.StatusCode == http.StatusOK && !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")}
	return resp, nil
}

type meterBody struct {
	io.ReadCloser
	meter                *CallMeter
	start                time.Time
	buf                  bytes.Buffer
	json, done, overflow bool
}

func (b *meterBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if b.json && !b.overflow {
		if b.buf.Len()+n > 8<<20 {
			b.buf.Reset()
			b.overflow = true
		} else {
			b.buf.Write(p[:n])
		}
	}
	if err != nil {
		b.finish(err == io.EOF)
	}
	return n, err
}

func (b *meterBody) Close() error {
	// json.Decoder may stop after the object without reading EOF.
	b.finish(b.json && json.Valid(b.buf.Bytes()))
	return b.ReadCloser.Close()
}

func (b *meterBody) finish(complete bool) {
	if b.done {
		return
	}
	b.done = true
	b.meter.add(b.buf.Bytes(), complete && b.json && !b.overflow, time.Since(b.start))
	b.buf.Reset()
}

func (m *CallMeter) add(body []byte, complete bool, elapsed time.Duration) {
	var response struct {
		Usage *struct {
			Input      *int `json:"input_tokens"`
			Output     *int `json:"output_tokens"`
			Prompt     *int `json:"prompt_tokens"`
			Completion *int `json:"completion_tokens"`
			CacheRead  int  `json:"cache_read_input_tokens"`
			CacheWrite int  `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.latency += elapsed.Milliseconds()
	if !complete || json.Unmarshal(body, &response) != nil || response.Usage == nil {
		return
	}
	u := response.Usage
	in, out := u.Prompt, u.Completion
	if m.anthropic {
		in, out = u.Input, u.Output
	}
	if in == nil || out == nil || *in < 0 || *out < 0 || u.CacheRead < 0 || u.CacheWrite < 0 {
		return
	}
	m.reported++
	m.input += *in
	m.output += *out
	if m.anthropic {
		m.cacheRead += u.CacheRead
		m.cacheWrite += u.CacheWrite
	}
}

func (t meterTransport) CloseIdleConnections() {
	if next, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		next.CloseIdleConnections()
	}
}
