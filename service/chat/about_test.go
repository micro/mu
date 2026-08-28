package chat

import (
	"os"
	"strings"
	"testing"
)

// One About block, and it folds, and it links to what the room is about.
//
// The page had two. This one above the messages, and a second built in
// JavaScript and inserted as the first thing inside #messages: the room's name
// again, the same summary again, a Hide summary link, and → View Original. So
// opening a room meant reading the same paragraph twice, a hundred milliseconds
// apart, with the room's title repeated between them.
//
// Reported as: chat summaries are above the message box and inside it, do it
// one way, and it has to keep the ability to hide the summary and view the
// original.
//
// So the two things the other copy had are the two things asserted hardest
// here, because deleting a duplicate is the easiest way to lose a feature that
// only lived in the copy you deleted.
func TestARoomSaysWhatItIsAboutOnce(t *testing.T) {
	got := aboutRoom(map[string]interface{}{
		"summary": "Three sentences about what people are discussing here.",
		"url":     "https://example.org/the-article",
	})

	if n := strings.Count(got, "Three sentences"); n != 1 {
		t.Errorf("the summary appears %d times in the About block, want once:\n%s", n, got)
	}
	// Foldable, by the element's own state rather than by a script.
	if !strings.HasPrefix(got, "<details") || !strings.Contains(got, "<summary>") {
		t.Errorf("the summary cannot be hidden — no <details> to fold it:\n%s", got)
	}
	if !strings.Contains(got, "open") {
		t.Errorf("the About block starts folded; it should be open, and foldable:\n%s", got)
	}
	// And the way back to what it is about.
	if !strings.Contains(got, `href="https://example.org/the-article"`) {
		t.Errorf("no link to the original:\n%s", got)
	}
	if !strings.Contains(got, `target="_blank"`) || !strings.Contains(got, "noopener") {
		t.Errorf("an outbound link opens in place, or hands the destination a "+
			"handle on this page:\n%s", got)
	}
}

// A summary is prose from somewhere else, so it is escaped.
//
// The copy this replaced wrote it through innerHTML. Room summaries are
// generated from indexed content — pages other people wrote — so that was a
// script tag away from being somebody else's JavaScript on this origin.
func TestARoomSummaryIsEscaped(t *testing.T) {
	got := aboutRoom(map[string]interface{}{
		"summary": `<script>alert(1)</script>`,
		"url":     "/blog/post",
	})
	if strings.Contains(got, "<script>") {
		t.Errorf("a room summary is rendered as markup:\n%s", got)
	}
}

// Somewhere on this instance is not somewhere else.
func TestALocalSourceIsNotOpenedInANewTab(t *testing.T) {
	got := aboutRoom(map[string]interface{}{"summary": "about a post", "url": "/blog/post"})
	if strings.Contains(got, "target=") {
		t.Errorf("a link to this instance opens a second tab:\n%s", got)
	}
}

// Nothing to say means nothing on the page, rather than an empty box with a
// fold control on it.
func TestARoomWithNothingToSaySaysNothing(t *testing.T) {
	if got := aboutRoom(map[string]interface{}{}); got != "" {
		t.Errorf("an empty About block was rendered: %q", got)
	}
	if got := aboutRoom(map[string]interface{}{"summary": "   "}); got != "" {
		t.Errorf("a blank summary rendered an About block: %q", got)
	}
}

// And the copy that was deleted stays deleted.
//
// The duplicate was in mu.js, not in this package, so nothing in these tests
// can see it come back. This can: the strings it was built from are distinctive
// and none of them has any other reason to exist.
func TestTheJavaScriptDoesNotBuildASecondAboutBlock(t *testing.T) {
	b, err := os.ReadFile("../../internal/app/html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, gone := range []string{"context-message", "summary-toggle", "View Original"} {
		if strings.Contains(src, gone) {
			t.Errorf("mu.js still builds the second About block (%q). What a room "+
				"is about is rendered once, by aboutRoom, above #messages.", gone)
		}
	}
}
