package social

import (
	"strings"
	"testing"
)

func TestDetectTruthSocial(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "apex https", url: "https://truthsocial.com/@mu/posts/123", want: true},
		{name: "www https", url: "https://www.truthsocial.com/@mu/posts/123", want: true},
		{name: "subdomain http", url: "http://media.truthsocial.com/@mu/posts/123", want: true},
		{name: "lookalike host", url: "https://nottruthsocial.com/@mu/posts/123", want: false},
		{name: "invalid", url: "://truthsocial.com/@mu", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectTruthSocial(tt.url); got != tt.want {
				t.Fatalf("DetectTruthSocial(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDetectSocialURLsIncludesMobileAndDeduplicates(t *testing.T) {
	content := strings.Join([]string{
		"source https://mobile.twitter.com/mu/status/1,",
		"mirror https://mobile.twitter.com/mu/status/1",
		"x post https://www.x.com/mu/status/2)",
		"truth https://truthsocial.com/@mu/posts/3.",
		"ignore https://example.com/mu/status/4",
	}, " ")

	got := DetectSocialURLs(content)
	want := []string{
		"https://mobile.twitter.com/mu/status/1",
		"https://www.x.com/mu/status/2",
		"https://truthsocial.com/@mu/posts/3",
	}
	if len(got) != len(want) {
		t.Fatalf("DetectSocialURLs() returned %d URLs, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DetectSocialURLs()[%d] = %q, want %q (all: %#v)", i, got[i], want[i], got)
		}
	}
}
