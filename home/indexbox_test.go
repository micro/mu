package home

// The front page draws the same box as everywhere else.
//
// It drew its own — .lsearch, its own input, its own arrow, its own stylesheet
// — beside app.SearchBox on Home. Two implementations of one control, which was
// survivable only while the control did one thing. The moment it grew a second
// button the copies would have had to be kept in step by hand. That button has
// since gone — asking answers in place, on the pages that ask — but the box
// stayed unified, which is the part that was worth doing.

import (
	"strings"
	"testing"
)

func TestTheFrontPageUsesTheOneSearchBox(t *testing.T) {
	got := indexBody()

	if !strings.Contains(got, `id="mu-search-input"`) {
		t.Errorf("the front page is not using app.SearchBox:\n%s", got)
	}
	// The old private copy, by the names only it had.
	for _, gone := range []string{"lsearch-in", "lsearch-go", `class="lsearch"`} {
		if strings.Contains(got, gone) {
			t.Errorf("the front page still carries its own search box (%s)", gone)
		}
	}
}
