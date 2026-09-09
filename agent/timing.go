package agent

import (
	gmagent "go-micro.dev/v6/agent"
	"mu/internal/app"
	"time"
)

func logRunTiming(e gmagent.RunEvent) {
	app.Log("timing", "phase=%s run=%s agent=%s name=%s provider=%s model=%s attempt=%d status=%s duration_ms=%d input_tokens=%d output_tokens=%d", e.Kind, e.RunID, e.Agent, e.Name, e.Provider, e.Model, e.Attempt, e.Status, e.LatencyMS, e.Tokens.InputTokens, e.Tokens.OutputTokens)
	if e.Kind != "model" && e.Kind != "stream" {
		return
	}
	app.RecordExternalCall(app.APILogEntry{Kind: "model", Time: e.Time, Service: e.Provider, Method: e.Kind, Model: e.Model, RunID: e.RunID, Outcome: e.Status, Attempt: e.Attempt, Duration: time.Duration(e.LatencyMS) * time.Millisecond, Error: e.ErrorKind, InputTokens: e.Tokens.InputTokens, OutputTokens: e.Tokens.OutputTokens})
}
