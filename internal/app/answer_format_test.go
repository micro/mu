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

func TestNormalizeAnswerMarkdownConvertsBulletGlyphs(t *testing.T) {
	input := "•   Weather is clear\n• Bring sunglasses"
	want := "- Weather is clear\n- Bring sunglasses"
	if got := NormalizeAnswerMarkdown(input); got != want {
		t.Fatalf("NormalizeAnswerMarkdown() = %q, want %q", got, want)
	}
}

func TestNormalizeAnswerMarkdownProtectsCurrencyDollars(t *testing.T) {
	input := "- BTC: $94,000\n- ETH: $4,703\n- PAXG: $3,942"
	got := NormalizeAnswerMarkdown(input)
	if strings.Contains(got, "$94") || strings.Contains(got, "$4") || strings.Contains(got, "$3") {
		t.Fatalf("currency dollars were not protected from math rendering: %q", got)
	}
	for _, want := range []string{"$\u206094,000", "$\u20604,703", "$\u20603,942"} {
		if !strings.Contains(got, want) {
			t.Fatalf("NormalizeAnswerMarkdown() missing %q in %q", want, got)
		}
	}
}
