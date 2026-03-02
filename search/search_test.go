package search

import "testing"

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
