package search

import (
	"net/url"
	"testing"
)

func TestResolveLinkNeutralizesUnsafeSchemes(t *testing.T) {
	base, err := url.Parse("https://example.com/articles/post")
	if err != nil {
		t.Fatal(err)
	}

	for _, href := range []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		" data:text/html,hello",
		"DATA:text/html,hello",
	} {
		if got := resolveLink(href, base); got != "#" {
			t.Errorf("resolveLink(%q) = %q; want #", href, got)
		}
	}
}

func TestResolveLinkPreservesSafeLinks(t *testing.T) {
	base, err := url.Parse("https://example.com/articles/post")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		href string
		want string
	}{
		{"/about", "https://example.com/about"},
		{"../archive", "https://example.com/archive"},
		{"#section", "#section"},
		{"mailto:hello@example.com", "mailto:hello@example.com"},
		{"TEL:+15551234567", "TEL:+15551234567"},
	}
	for _, tc := range tests {
		if got := resolveLink(tc.href, base); got != tc.want {
			t.Errorf("resolveLink(%q) = %q; want %q", tc.href, got, tc.want)
		}
	}
}

func TestIsProxyableLinkHandlesSchemesCaseInsensitively(t *testing.T) {
	for _, href := range []string{"https://example.com", "HTTP://example.com"} {
		if !isProxyableLink(href) {
			t.Errorf("isProxyableLink(%q) = false; want true", href)
		}
	}
	for _, href := range []string{"javascript:alert(1)", "JavaScript:alert(1)", "data:text/html,hi", "mailto:hello@example.com", "#section"} {
		if isProxyableLink(href) {
			t.Errorf("isProxyableLink(%q) = true; want false", href)
		}
	}
}
