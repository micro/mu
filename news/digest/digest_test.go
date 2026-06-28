package digest

import (
	"strings"
	"testing"
)

func TestBuildReferences(t *testing.T) {
	got := buildReferences([]ref{
		{title: "First story", url: "https://example.com/first"},
		{title: "Second story", url: "https://example.com/second"},
	})

	wantParts := []string{
		"<details>",
		"<summary>References</summary>",
		"1. [First story](https://example.com/first)",
		"2. [Second story](https://example.com/second)",
		"</details>",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("buildReferences() missing %q in %q", part, got)
		}
	}
}

func TestBuildReferencesEmpty(t *testing.T) {
	if got := buildReferences(nil); got != "" {
		t.Fatalf("buildReferences(nil) = %q, want empty string", got)
	}
}

func TestStripPreamble(t *testing.T) {
	input := "Here is today's briefing:\n\nThe main story starts here.\nMarkets followed."
	want := "The main story starts here.\nMarkets followed."
	if got := stripPreamble(input); got != want {
		t.Fatalf("stripPreamble() = %q, want %q", got, want)
	}
}

func TestStripPreamblePreservesMarkdownHeading(t *testing.T) {
	input := "# Daily notes\nText follows."
	if got := stripPreamble(input); got != input {
		t.Fatalf("stripPreamble() = %q, want %q", got, input)
	}
}

func TestNormalizeHeadingsAddsBlankLineBeforeBody(t *testing.T) {
	input := "## Market moves\nOil rose.\n\n### Elsewhere\nGold fell."
	want := "## Market moves\n\nOil rose.\n\n### Elsewhere\n\nGold fell."
	if got := normalizeHeadings(input); got != want {
		t.Fatalf("normalizeHeadings() = %q, want %q", got, want)
	}
}

func TestCleanResponse(t *testing.T) {
	input := "Below is the digest:\n\n## Markets\nOil moved from $90 to $94."
	got := cleanResponse(input)
	if strings.Contains(strings.ToLower(got), "below is") {
		t.Fatalf("cleanResponse() kept preamble: %q", got)
	}
	if !strings.Contains(got, "## Markets\n\nOil moved") {
		t.Fatalf("cleanResponse() did not normalize heading spacing: %q", got)
	}
}
