package web

import (
	"strings"
	"testing"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"no tags here", "no tags here"},
		{"<strong>bold</strong> text", "bold text"},
		{"<em>italic</em> and <b>bold</b>", "italic and bold"},
		{"result with &amp; entity", "result with & entity"},
		{"<b>hello</b> &lt;world&gt;", "hello <world>"},
		{"", ""},
	}
	for _, tc := range tests {
		got := stripHTML(tc.input)
		if got != tc.want {
			t.Errorf("stripHTML(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestRecentSearchesUseSharedComponent(t *testing.T) {
	if !strings.Contains(webRecentSearchesScript, `data-recent-searches="web-search"`) || !strings.Contains(webRecentSearchesScript, `data-storage-key="mu_recent_web_searches"`) {
		t.Fatal("recent searches must use the shared form-bound component")
	}
}
