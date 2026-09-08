package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProviderFailureMessages(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), errors.New("agent: ai generate failed after 3 attempt(s) (timeout): context deadline exceeded; agent provider call timed out; inspect run history with `micro inspect agent <name> --status timeout`, then adjust AgentModelCallTimeout/AgentModelRetry or see docs/guides/debugging-agents.md")} {
		got := FailureMessage(err)
		if !strings.Contains(got, "too long") || strings.Contains(got, "micro inspect") || strings.Contains(got, "AgentModel") {
			t.Fatal(got)
		}
	}
	if got := FailureMessage(errors.New("Insufficient credits")); got != "Insufficient credits" {
		t.Fatal(got)
	}
	if FailureMessage(nil) != "" {
		t.Fatal("nil failure")
	}
}
