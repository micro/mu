package agent

import (
	gmagent "go-micro.dev/v6/agent"
	"mu/internal/app"
	"testing"
	"time"
)

func TestModelTimingIncludesAttemptsAndFailures(t *testing.T) {
	for _, status := range []string{"done", "timeout"} {
		logRunTiming(gmagent.RunEvent{Kind: "model", RunID: "timing-test", Provider: "test", Model: "test-model", Status: status, Attempt: 2, LatencyMS: 123, Tokens: gmagent.Usage{InputTokens: 9, OutputTokens: 3}})
		got := app.APILog()[0]
		if got.Kind != "model" || got.RunID != "timing-test" || got.Outcome != status || got.Attempt != 2 || got.Duration != 123*time.Millisecond || got.InputTokens != 9 || got.OutputTokens != 3 {
			t.Fatalf("bad model timing: %+v", got)
		}
		if got.Status != 0 {
			t.Fatal("model event invented an HTTP status")
		}
	}
}

func TestModelTimingRetainsProviderError(t *testing.T) {
	logRunTiming(gmagent.RunEvent{Kind: "model", ErrorKind: "rate_limited", Error: "HTTP 429 quota=RequestsPerDay limit=0 retryDelay=60s"})
	got := app.APILog()[0]
	if got.Outcome != "rate_limited" || got.Error != "HTTP 429 quota=RequestsPerDay limit=0 retryDelay=60s" {
		t.Fatalf("lost provider diagnostic: %+v", got)
	}
}

func TestModelLogUsesSelectedRoute(t *testing.T) {
	router := runTimingFor("openrouter")
	direct := runTimingFor("openai")
	for _, kind := range []string{"model", "stream"} {
		router(gmagent.RunEvent{Kind: kind, Provider: "openai", Model: "poolside/laguna", Status: "done"})
		if got := app.APILog()[0]; got.Service != "openrouter" || got.Model != "poolside/laguna" {
			t.Fatalf("wrong route: %+v", got)
		}
		direct(gmagent.RunEvent{Kind: kind, Provider: "openai", Status: "done"})
		if got := app.APILog()[0]; got.Service != "openai" {
			t.Fatalf("direct route relabeled: %+v", got)
		}
	}
}
