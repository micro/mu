package search

import (
	"strings"
	"testing"
)

func TestFormatWebSearchResultsPreservesIntentAndSources(t *testing.T) {
	got := formatWebSearchResults("micro mu", []BraveResult{
		{Title: "Mu — an everyday agent", Description: "Mu is built by Micro as an agent for everyday internet tasks.", URL: "https://micro.mu/"},
		{Title: "GitHub - micro/mu", Description: "Source repository for Mu.", URL: "https://github.com/micro/mu"},
	})

	for _, want := range []string{
		`Web results for "micro mu"`,
		`Query intent: answer the user's original query "micro mu"`,
		"Confidence: high",
		"Sources:",
		"1. Mu — an everyday agent",
		"https://micro.mu/",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in formatted web search results:\n%s", want, got)
		}
	}
}

func TestFormatWebSearchResultsFlagsLowConfidenceMismatch(t *testing.T) {
	got := formatWebSearchResults("micro mu", []BraveResult{
		{Title: "Micro-", Description: "Micro is a metric prefix meaning one millionth.", URL: "https://example.com/micro-prefix"},
	})

	for _, want := range []string{
		"Confidence: low",
		"only partially match the query intent",
		"ask the user to refine the query before making unsupported claims",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in low-confidence web search results:\n%s", want, got)
		}
	}
}
