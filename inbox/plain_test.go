package inbox

// A preview has no renderer, so it gets the words.
//
// Reported from Home. The agent answered in markdown and the row showed it
// literally:
//
//	@micro ?
//	Agent **@micro** is an agent on this instance. - **Na
//
// The asterisks, the bullet, and a cut landing mid-word inside one. The thread
// below renders the markdown properly — this is the one line above it, escaped
// and drawn as text, and what belongs in it is the sentence.

import (
	"strings"
	"testing"
)

func TestAPreviewHasNoMarkdownInIt(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// The reported case.
		{"**@micro** is an agent on this instance.\n\n- **Name**: micro",
			"@micro is an agent on this instance. Name: micro"},
		{"# Heading\nThen the text.", "Heading Then the text."},
		{"1. First\n2. Second", "First Second"},
		{"> quoted line", "quoted line"},
		{"Read [the docs](https://example.com/docs) first.", "Read the docs first."},
		{"Use `go test ./...` to check.", "Use go test ./... to check."},
		{"*emphasis* and __strong__ and normal.", "emphasis and strong and normal."},

		// And what is not a mark stays. A dash mid-sentence is a dash, a hash
		// in "#1" is a hash: a preview that mangles an ordinary message is
		// worse than one that shows a stray character. A single underscore is
		// left alone for the same reason — snake_case is far more common in
		// anything this instance carries than _emphasis_ is.
		{"It cost 5 - 10 dollars, see #1 above.", "It cost 5 - 10 dollars, see #1 above."},
		{"snake_case_name stays whole", "snake_case_name stays whole"},
		{"plain text, untouched", "plain text, untouched"},
	} {
		if got := trimTo(c.in, 200); got != c.want {
			t.Errorf("trimTo(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// And the trim still trims, on a word, after the marks come off.
//
// The order matters: cutting first and stripping second leaves a message
// ending in half a mark, which is the "**Na" in the report.
func TestTheCutHappensAfterTheMarksComeOff(t *testing.T) {
	long := "**" + strings.Repeat("word ", 40) + "**"
	got := trimTo(long, 30)

	if strings.Contains(got, "*") {
		t.Errorf("a mark survived the trim: %q", got)
	}
	if len([]rune(got)) > 31 { // 30 plus the ellipsis
		t.Errorf("trimTo(_, 30) returned %d characters: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a trimmed line does not say it was trimmed: %q", got)
	}
}
