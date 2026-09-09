package app

import (
	"regexp"
	"strings"
	"testing"
)

func TestApplicationPagesShareOneFrame(t *testing.T) {
	b, err := htmlFiles.ReadFile("html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(b), "")
	if strings.Count(css, "--page-width:") != 1 {
		t.Fatal("page width must be defined once")
	}
	for _, rule := range regexp.MustCompile(`[^{}]+\{[^{}]*\}`).FindAllString(css, -1) {
		selector, body, _ := strings.Cut(rule, "{")
		// Kiosk mode explicitly removes the navigation shell.
		if strings.TrimSpace(selector) == "body.display-mode #content" {
			continue
		}
		if strings.Contains(selector, "#content") && strings.Contains(body, "max-width:") && !strings.Contains(selector, ".card") {
			if strings.TrimSpace(selector) != "#content" || !strings.Contains(body, "max-width: var(--page-width)") {
				t.Errorf("page-specific shell width: %s", rule)
			}
		}
	}
	for _, selector := range []string{".page-col", ".card", ".col-feed", ".markets-page", ".browser-page", ".maps-page"} {
		pattern := regexp.MustCompile(regexp.QuoteMeta(selector) + `\s*\{[^}]*max-width:\s*var\(--page-width\)`)
		if !pattern.MatchString(css) {
			t.Errorf("%s does not use the shared width", selector)
		}
	}
	if strings.Contains(css, "#content:has(.page-col) #page-title") {
		t.Fatal("page type moves title independently")
	}
}

func TestHomeSessionCannotInjectLegacyLayout(t *testing.T) {
	b, err := htmlFiles.ReadFile("html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"initCardCustomization", "customize-link", "mu_hidden_cards", "pageTitle.parentNode.insertBefore"} {
		if strings.Contains(string(b), old) {
			t.Errorf("legacy home DOM mutation remains: %s", old)
		}
	}
}
