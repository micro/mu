package app

import (
	"mu/internal/result"
	"strings"
	"testing"
)

func TestWidgetUsesOnlyLocalAppIdentity(t *testing.T) {
	html := Results([]result.Item{{Kind: "app", ID: "private-widget", URL: "https://evil.example", Title: "<script>"}})
	if !strings.Contains(html, `src="/apps/private-widget?widget=1"`) || strings.Contains(html, "evil.example") || strings.Contains(html, "<script>") || strings.Contains(html, "<details") {
		t.Fatal(html)
	}
	for _, id := range []string{"../account", "x\" onload=\"alert(1)", "https://evil.example"} {
		if strings.Contains(Results([]result.Item{{Kind: "app", ID: id}}), "iframe") {
			t.Fatal("unsafe app identity accepted")
		}
	}
}

func TestSavedContentAndVideoAreVisibleAndSafe(t *testing.T) {
	for _, kind := range []string{"note", "doc"} {
		body := Results([]result.Item{{Kind: kind, ID: "saved-id", Title: "Saved", Body: "First\nSecond\n\n<script>alert(1)</script>\n\n![remote](https://example.com/private.png)"}})
		if strings.Contains(body, "<details") || !strings.Contains(body, "First<br>") || strings.Contains(body, "<script>") || strings.Contains(body, "<img") {
			t.Fatal(body)
		}
	}
	body := Results([]result.Item{{Kind: "video", ID: "abc123", Title: "A video"}})
	if strings.Contains(body, "<details") || !strings.Contains(body, "youtube.com/embed/abc123") {
		t.Fatal(body)
	}
}
