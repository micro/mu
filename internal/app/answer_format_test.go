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
