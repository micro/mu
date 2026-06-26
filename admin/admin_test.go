package admin

import (
	"strings"
	"testing"
)

func TestBlocklistRowsEscapeValues(t *testing.T) {
	tests := []struct {
		name string
		html string
	}{
		{name: "email", html: blocklistEmailRow(`bad"><script>alert(1)</script>@example.com`)},
		{name: "ip", html: blocklistIPRow(`127.0.0.1"><script>alert(1)</script>`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.html, `<script>`) || strings.Contains(tt.html, `"><script>`) {
				t.Fatalf("blocklist row contains unescaped value: %s", tt.html)
			}
			if !strings.Contains(tt.html, `&lt;script&gt;alert(1)&lt;/script&gt;`) {
				t.Fatalf("blocklist row does not include escaped script text: %s", tt.html)
			}
			if !strings.Contains(tt.html, `&#34;&gt;`) {
				t.Fatalf("blocklist row does not escape attribute-breaking quote: %s", tt.html)
			}
		})
	}
}
