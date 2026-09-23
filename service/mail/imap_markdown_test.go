package mail

import (
	"strings"
	"testing"
)

func TestGeneratedBridgeMarkdownIsRendered(t *testing.T) {
	body, kind := imapTextBody(&Message{Bridged: true, Markdown: true, Body: "**Done**\n\n[Open app](https://example.com/app)\n\n<script>alert(1)</script>"})
	if kind != "HTML" || strings.Contains(body, "**Done**") || !strings.Contains(body, "<strong>Done</strong>") || strings.Contains(body, "<script>") {
		t.Fatalf("unsafe or raw message: %s %s", kind, body)
	}
	body, kind = imapTextBody(&Message{Bridged: true, Body: "<b>literal chat text</b>"})
	if kind != "PLAIN" || !strings.Contains(body, "<b>") {
		t.Fatal("ordinary chat text treated as HTML")
	}
}
