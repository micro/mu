package search

import (
	"net/http"
	"net/url"
	"strings"
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

func TestIsPrivateHostBlocksInternalAddresses(t *testing.T) {
	for _, host := range []string{
		"localhost",
		"127.0.0.1",
		"10.1.2.3",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.10",
		"169.254.169.254",
		"0.0.0.0",
		"::1",
		"[::1]",
		"fe80::1",
		"::ffff:127.0.0.1",
		"metadata.google.internal",
	} {
		if !isPrivateHost(host) {
			t.Errorf("isPrivateHost(%q) = false; want true", host)
		}
	}
}

func TestIsPrivateHostAllowsPublicHosts(t *testing.T) {
	for _, host := range []string{
		"example.com",
		"8.8.8.8",
		"1.1.1.1",
		"2001:4860:4860::8888",
	} {
		if isPrivateHost(host) {
			t.Errorf("isPrivateHost(%q) = true; want false", host)
		}
	}
}

func TestFetchAndExtractRejectsPrivateURLsBeforeRequest(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1/private",
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
	} {
		_, _, err := FetchAndExtract(rawURL)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "private or internal") {
			t.Fatalf("FetchAndExtract(%q) error = %v; want private/internal rejection", rawURL, err)
		}
	}
}

func TestFetchClientRejectsRedirectsToPrivateURLs(t *testing.T) {
	redirectURL, err := url.Parse("http://127.0.0.1/admin")
	if err != nil {
		t.Fatal(err)
	}
	publicURL, err := url.Parse("https://example.com/start")
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{URL: redirectURL}
	via := []*http.Request{{URL: publicURL}}

	err = fetchClient.CheckRedirect(req, via)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "private or internal") {
		t.Fatalf("CheckRedirect() error = %v; want private/internal rejection", err)
	}
}
