package search

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

func TestRecentSearchesScriptEscapesLabelsWithoutManglingSpaces(t *testing.T) {
	if !strings.Contains(webRecentSearchesScript, "div.textContent = String(text);") {
		t.Fatal("recent search labels should escape via textContent")
	}
	if strings.Contains(webRecentSearchesScript, ".replace(/ /g, '&gt;')") || strings.Contains(webRecentSearchesScript, ".replace(/\\s/g, '&gt;')") {
		t.Fatal("recent search escaping must not convert spaces to HTML entities")
	}
	if !strings.Contains(webRecentSearchesScript, "encodeURIComponent(search)") {
		t.Fatal("recent search data attributes should encode raw queries without changing label text")
	}
	if !strings.Contains(webRecentSearchesScript, "decodeURIComponent(item.getAttribute('data-query') || '')") {
		t.Fatal("recent search click/remove handlers should decode stored queries")
	}
}
