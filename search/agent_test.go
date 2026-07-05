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

func TestFormatWebSearchResultsPrefersArticleLevelNewsSources(t *testing.T) {
	got := formatWebSearchResults("today's AI news", []BraveResult{
		{Title: "The Information", Description: "Reporting on AI startup funding and model launches this week.", URL: "https://www.theinformation.com/"},
		{Title: "AI News, Updates, Products and Reviews | Yahoo Tech", Description: "Meta is preparing new AI glasses as rivals race to ship wearable assistants.", URL: "https://www.yahoo.com/tech/ai/"},
		{Title: "AI (artificial intelligence) | The Guardian", Description: "Coverage of AI policy and product launches from around the world.", URL: "https://www.theguardian.com/technology/artificialintelligenceai"},
		{Title: "Artificial Intelligence News | Bloomberg", Description: "Latest artificial intelligence stories, analysis and market coverage.", URL: "https://www.bloomberg.com/technology/ai"},
		{Title: "Meta prepares AI glasses for developers", Description: "Meta plans to show AI glasses to developers today, according to people familiar with the launch.", URL: "https://example.com/2026/07/04/meta-ai-glasses-developers"},
	})

	if strings.Contains(got, "The Information") || strings.Contains(got, "Yahoo Tech") || strings.Contains(got, "The Guardian") || strings.Contains(got, "Bloomberg") {
		t.Fatalf("expected generic/root AI-news sources to be dropped when article-level stories exist:\n%s", got)
	}
	if !strings.Contains(got, "Meta prepares AI glasses for developers") {
		t.Fatalf("expected article-level AI-news source to remain:\n%s", got)
	}
}

func TestFormatWebSearchResultsLabelsGenericNewsSourcesAsLimitedEvidence(t *testing.T) {
	got := formatWebSearchResults("today's AI news", []BraveResult{
		{Title: "The Information", Description: "Reporting on AI startup funding and model launches this week.", URL: "https://www.theinformation.com/"},
		{Title: "AI News, Updates, Products and Reviews | Yahoo Tech", Description: "Meta is preparing new AI glasses as rivals race to ship wearable assistants.", URL: "https://www.yahoo.com/tech/ai/"},
		{Title: "AI (artificial intelligence) | The Guardian", Description: "Coverage of AI policy and product launches from around the world.", URL: "https://www.theguardian.com/technology/artificialintelligenceai"},
	})

	for _, unwanted := range []string{
		"1. The Information —",
		"AI News, Updates, Products and Reviews | Yahoo Tech",
		"AI (artificial intelligence) | The Guardian",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("expected generic source labels to be replaced with limited-evidence labels, still found %q:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{
		"Limited evidence from www.theinformation.com",
		"Limited evidence from www.yahoo.com",
		"If the snippets are thin, say the evidence is limited.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in limited-evidence web search results:\n%s", want, got)
		}
	}
}
