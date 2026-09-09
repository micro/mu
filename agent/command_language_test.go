package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mu/internal/service"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/users"
	"mu/service/video"
	"mu/service/weather"
)

type commandNetworkSpy struct{ calls atomic.Int32 }

func (s *commandNetworkSpy) RoundTrip(*http.Request) (*http.Response, error) {
	s.calls.Add(1)
	return nil, fmt.Errorf("network forbidden in direct command test")
}

func TestNaturalCommandsNeverCallAModel(t *testing.T) {
	spy := new(commandNetworkSpy)
	old := http.DefaultTransport
	http.DefaultTransport = spy
	t.Cleanup(func() { http.DefaultTransport = old })
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("OPENAI_BASE_URL", "https://model.invalid/v1")
	for _, spec := range []service.Spec{news.Spec, weather.Spec, markets.Spec, video.Spec, users.Spec} {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ input, svc string }{
		{"News today", "news"}, {"Please show me news", "news"}, {"markets", "markets"}, {"latest videos", "video"}, {"users", "users"}, {"weather in London tomorrow", "weather"},
	} {
		calls, ok := promptCommands(tc.input, QueryOpts{})
		if !ok || len(calls) != 1 || calls[0].Service != tc.svc {
			t.Errorf("%q: %+v %v", tc.input, calls, ok)
		}
	}
	for _, prompt := range []string{"News today", "please news today", "weather tomorrow", "weather and news today", "/weather and news today", "/help", "/nonexistent-command", "users"} {
		text, err := QueryWithOpts("", prompt, QueryOpts{Public: true})
		if prompt == "/nonexistent-command" || prompt == "users" {
			if err == nil {
				t.Fatal("unknown explicit command guessed")
			}
			continue
		}
		if err != nil || text == "" {
			t.Errorf("%q: %q %v", prompt, text, err)
		}
	}
	if spy.calls.Load() != 0 {
		t.Fatalf("direct route made %d network/model requests", spy.calls.Load())
	}
	if _, ok := promptCommands("news and weather", QueryOpts{Extra: "interpret this attachment"}); ok {
		t.Fatal("discarded attachment")
	}
	if _, ok := promptCommands("/news and weather", QueryOpts{Extra: "attachment"}); !ok {
		t.Fatal("explicit command lost")
	}
	if _, ok := promptCommands("users", QueryOpts{Public: true}); ok {
		t.Fatal("guest directory access")
	}
}

type ParallelReadProbe struct {
	started chan string
	release chan struct{}
}
type ParallelReadRequest struct {
	Value string `json:"value"`
}

func (p *ParallelReadProbe) Read(ctx context.Context, req *ParallelReadRequest, rsp *CommandProbeResponse) error {
	select {
	case p.started <- req.Value:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-p.release:
		rsp.Text = req.Value
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestComposedCommandsExecuteConcurrentlyAndRenderInOrder(t *testing.T) {
	p := &ParallelReadProbe{started: make(chan string, 2), release: make(chan struct{})}
	if err := service.Register(service.Spec{Name: "parallelreads", Handler: p, Endpoints: map[string]service.Endpoint{"Read": {}}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan string, 1)
	var starts, ends, steps int
	ids := map[string]bool{}
	go func() {
		text, err := executeCommands(ctx, "", []service.CommandCall{{Service: "parallelreads", Method: "Read", Args: map[string]any{"value": "First result"}}, {Service: "parallelreads", Method: "Read", Args: map[string]any{"value": "Second result"}}}, QueryOpts{Stream: StreamHooks{ToolStart: func(r ToolRun) { starts++; ids[r.ID] = true }, ToolEnd: func(ToolRun) { ends++ }}, OnStep: func(Step) { steps++ }})
		if err != nil {
			text = err.Error()
		}
		result <- text
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-p.started:
		case <-ctx.Done():
			t.Fatal("reads did not start in parallel")
		}
	}
	close(p.release)
	text := <-result
	if !strings.Contains(text, "First result") || strings.Index(text, "First result") > strings.Index(text, "Second result") || starts != 2 || ends != 2 || steps != 2 || len(ids) != 2 {
		t.Fatalf("%q lifecycle=%d/%d/%d", text, starts, ends, steps)
	}
}

func TestComposedCommandFailureIsNotASecondModelAttempt(t *testing.T) {
	if err := service.Register(service.Spec{Name: "composedfailure", Handler: CommandProbe{}, Endpoints: map[string]service.Endpoint{"Read": {}}}); err != nil {
		t.Fatal(err)
	}
	for _, allFail := range []bool{false, true} {
		second := "success"
		if allFail {
			second = "fail"
		}
		text, err := executeCommands(context.Background(), "owner", []service.CommandCall{{Service: "composedfailure", Method: "Read", Args: map[string]any{"mode": "fail"}}, {Service: "composedfailure", Method: "Read", Args: map[string]any{"mode": second}}}, QueryOpts{})
		if allFail {
			if err == nil {
				t.Fatal("all failed marked success")
			}
		} else if err != nil || !strings.Contains(text, "provider unavailable") || !strings.Contains(text, "Ready for owner") {
			t.Fatalf("lost partial outcome: %q %v", text, err)
		}
	}
}

func TestDirectListsRenderTheirRegisteredResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		result map[string]any
		want   string
	}{
		{map[string]any{"notes": []any{map[string]any{"title": "School run", "text": "Leave at eight"}}}, "School run — Leave at eight"},
		{map[string]any{"notes": nil}, "No results."},
		{map[string]any{"entries": []any{map[string]any{"text": "Alice is home", "at": "2026-09-09T08:00:00Z"}}}, "Alice is home"},
		{map[string]any{"entries": []any{}}, "No results."},
		{map[string]any{"events": "Tomorrow at eight: school run"}, "Tomorrow at eight"},
		{map[string]any{"files": []any{map[string]any{"name": "Family photo"}}}, "Family photo"},
	} {
		if got := commandText(tc.result); !strings.Contains(got, tc.want) {
			t.Fatalf("%v rendered as %q", tc.result, got)
		}
	}
}
