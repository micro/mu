package app

import (
	"html"
	"strings"
	"testing"
)

func TestLinkifyMakesAURLClickable(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"see https://micro.mu/tools",
			`see <a href="https://micro.mu/tools" rel="noopener noreferrer">https://micro.mu/tools</a>`},
		// A URL at the end of a sentence: the stop belongs to the sentence.
		{"go to https://micro.mu/tools.",
			`go to <a href="https://micro.mu/tools" rel="noopener noreferrer">https://micro.mu/tools</a>.`},
		// In brackets, which is how people paste them.
		{"(https://micro.mu)",
			`(<a href="https://micro.mu" rel="noopener noreferrer">https://micro.mu</a>)`},
		// Nothing to do.
		{"no links here", "no links here"},
		// Not a link: a bare domain is prose, and guessing turns a sentence
		// into a link that goes nowhere.
		{"write to mu.test for details", "write to mu.test for details"},
		// Not mid-word.
		{"xhttps://micro.mu", "xhttps://micro.mu"},
	} {
		if got := Linkify(c.in); got != c.want {
			t.Errorf("Linkify(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// The ordering is the safety property: escaped in, HTML out. Linkifying raw
// text would let markup through in the href.
func TestLinkifyIsSafeOnEscapedText(t *testing.T) {
	raw := `<script>alert(1)</script> https://micro.mu/x?a=1&b=2`
	got := Linkify(html.EscapeString(raw))

	if strings.Contains(got, "<script>") {
		t.Errorf("markup survived: %s", got)
	}
	// The query separator arrives escaped and has to stay that way inside the
	// href — an unescaped & in an attribute is what the escaper was for.
	if !strings.Contains(got, `href="https://micro.mu/x?a=1&amp;b=2"`) {
		t.Errorf("the query string did not survive the link:\n%s", got)
	}
}
