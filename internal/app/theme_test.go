package app

import (
	"strings"
	"testing"
)

// Both palettes must declare their browser control colour scheme. Otherwise
// native inputs can be repainted independently of the surrounding application.
func TestTheStylesheetDeclaresItsTheme(t *testing.T) {
	b, err := htmlFiles.ReadFile("html/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, declaration := range []string{"color-scheme: light", ".dark {", "color-scheme: dark"} {
		if !strings.Contains(css, declaration) {
			t.Errorf("shared theme is missing %q", declaration)
		}
	}
	if !strings.Contains(Styles(), "--background:") || !strings.Contains(Styles(), "--font-sans:") || !strings.Contains(Styles(), ".dark{") {
		t.Error("legacy pages do not receive the shared theme")
	}
}
