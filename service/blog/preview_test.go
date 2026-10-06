package blog

import (
	"html"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreviewText(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"# Heading\n\nFirst **bold** paragraph with [a link](https://example.test).\n\nSecond paragraph.", "First bold paragraph with a link."},
		{"Hello *世界* & `code`\n\nMore", "Hello 世界 &amp; code"},
		{strings.Repeat("界", 300), strings.Repeat("界", 279) + "…"},
	} {
		got := previewText(tt.input)
		if got != tt.want {
			t.Errorf("preview = %q; want %q", got, tt.want)
		}
		if utf8.RuneCountInString(html.UnescapeString(got)) > 280 {
			t.Fatal("preview exceeds limit")
		}
	}
}
