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

func TestFormatWebSearchResultsGroundsNewsFallbackInSnippets(t *testing.T) {
	got := formatWebSearchResults("latest AI news", []BraveResult{
		{Title: "Artificial Intelligence News", Description: "", URL: "https://example.com/ai"},
		{Title: "AI chip supplier expands capacity", Description: "A chip supplier said demand from AI data centers is rising this quarter.", URL: "https://example.com/ai-chips"},
		{Title: "Sports scores", Description: "Local football results from last night.", URL: "https://example.com/sports"},
	})

	if strings.Contains(got, "Artificial Intelligence News") || strings.Contains(got, "Sports scores") {
		t.Fatalf("expected weak or unrelated news fallback sources to be filtered:\n%s", got)
	}
	for _, want := range []string{
		"AI chip supplier expands capacity",
		"Grounding rule:",
		"do not turn generic topic/category pages or weak snippets into specific news headlines",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in grounded web search results:\n%s", want, got)
		}
	}
}
