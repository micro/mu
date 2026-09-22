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
