package app

import (
	"strings"
	"testing"
)

func TestNormalizeAnswerMarkdownCollapsesSpacing(t *testing.T) {
	input := "  ### Weather  \r\n\r\n\r\n-  Sunny today  \n*   Bring sunglasses\n\n\nDone.  "
	want := "### Weather\n\n- Sunny today\n* Bring sunglasses\n\nDone."
	if got := NormalizeAnswerMarkdown(input); got != want {
		t.Fatalf("NormalizeAnswerMarkdown() = %q, want %q", got, want)
	}
}

func TestNormalizeAnswerMarkdownPreservesCodeFenceSpacing(t *testing.T) {
	input := "Before\n\n```\n  keep indentation  \n\nnext\n```\n\n\nAfter"
	got := NormalizeAnswerMarkdown(input)
	if !strings.Contains(got, "```\n  keep indentation\n\nnext\n```") {
		t.Fatalf("code fence content was not preserved: %q", got)
	}
	if strings.Contains(got, "After\n\n\n") {
		t.Fatalf("extra blank lines leaked outside code fence: %q", got)
	}
}

func TestNormalizeAnswerMarkdownNormalizesCommonBlocks(t *testing.T) {
	input := "##Weather\n1.Sunny today\n>quoted\n| City | Temp|Notes |\n|---| --- |---|"
	want := "## Weather\n1. Sunny today\n> quoted\n| City | Temp | Notes |\n| --- | --- | --- |"
	if got := NormalizeAnswerMarkdown(input); got != want {
		t.Fatalf("NormalizeAnswerMarkdown() = %q, want %q", got, want)
	}
}

func TestNormalizeAnswerMarkdownRepairsMalformedLeadingBold(t *testing.T) {
	input := "*London today:** Light rain, 17°C.\n*Wind:** 10 mph."
	want := "**London today:** Light rain, 17°C.\n**Wind:** 10 mph."
	if got := NormalizeAnswerMarkdown(input); got != want {
		t.Fatalf("NormalizeAnswerMarkdown() = %q, want %q", got, want)
	}
}

func TestRenderMalformedLeadingBoldAsCleanStrongText(t *testing.T) {
	input := NormalizeAnswerMarkdown("*London today:** Light rain, 17°C.")
	html := string(Render([]byte(input)))
	if strings.Contains(html, "*") {
		t.Fatalf("rendered HTML leaked markdown delimiter: %q", html)
	}
	if !strings.Contains(html, "<strong>London today:</strong>") {
		t.Fatalf("rendered HTML did not produce strong label: %q", html)
	}
}

func TestNormalizeAnswerMarkdownDoesNotTreatEmphasisAsList(t *testing.T) {
	input := "*emphasis* stays inline\n* list item"
	want := "*emphasis* stays inline\n* list item"
	if got := NormalizeAnswerMarkdown(input); got != want {
		t.Fatalf("NormalizeAnswerMarkdown() = %q, want %q", got, want)
	}
}
