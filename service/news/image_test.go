package news

// Where an article's picture comes from.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every image this instance shows is served by this instance.
//
// The article page was the one that still hotlinked, and internal/imageproxy's
// package comment describes what happens to anything that does — in the words
// of the bug reported against this page: "when one says no the image is simply
// gone, and the onerror handler hides it, so the page looks like it never had a
// picture". A hotlinked image is at the mercy of the publisher's hotlink rules,
// a resource policy, a content blocker's list, an expiring signed URL, or a
// rate limit, and any one of them turns a page that had a picture yesterday
// into one that does not.
//
// Asserted against the source, because the alternative is rendering a page and
// then deciding what its img tags mean — and what is being pinned is where the
// URL came from, which only the source says.
func TestNoImageIsHotlinked(t *testing.T) {
	b, err := os.ReadFile("news.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	// Every img tag whose src is a format verb: those are the ones filled in
	// with a URL at render time, and each one has to be filled from the proxy.
	tags := regexp.MustCompile(`<img[^>]*src="%s"[^>]*>`).FindAllString(src, -1)
	if len(tags) < 2 {
		t.Fatalf("found %d rendered img tags — this scan is broken, not the code", len(tags))
	}
	for _, tag := range tags {
		// The call that fills it is on the same statement, which is the line or
		// two after the tag. Close enough to find, far enough to be readable.
		at := strings.Index(src, tag)
		window := src[at:min(at+400, len(src))]
		if !strings.Contains(window, "imageproxy.URL(") {
			t.Errorf("this image is hotlinked from the publisher rather than served "+
				"from here:\n%s", tag)
		}
	}
}

// And nothing asks the publisher not to be told who is reading, because
// nothing asks the publisher anything.
func TestTheReferrerDodgeIsGone(t *testing.T) {
	b, err := os.ReadFile("news.go")
	if err != nil {
		t.Fatal(err)
	}
	// The attribute in markup, not the word. A scan for the word matches the
	// comment explaining why the attribute is gone, which is the test failing
	// on its own prose.
	if strings.Contains(string(b), `referrerpolicy="`) {
		t.Error("a referrerpolicy is still being set, which only matters on an " +
			"image fetched from somebody else — see internal/imageproxy")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
