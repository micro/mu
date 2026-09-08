package ai

import (
	"context"
	"errors"
	"strings"
)

// FailureMessage translates provider failures for product surfaces. Detailed
// diagnostics belong in logs and run history, not CLI advice in a task reply.
func FailureMessage(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	lower := strings.ToLower(text)
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(lower, "context deadline exceeded"), strings.Contains(lower, "provider call timed out"), strings.Contains(lower, "(timeout)"):
		return "The model took too long to respond. This run stopped before completing."
	case errors.Is(err, context.Canceled):
		return "This run was cancelled before completing."
	}
	if i := strings.Index(text, "; inspect run history"); i >= 0 {
		text = text[:i]
	}
	if strings.Contains(text, "docs/guides/debugging-agents.md") {
		return "The model could not complete this run. Please try again later."
	}
	return text
}
