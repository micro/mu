package agent

import (
	"regexp"
	"strings"
	"testing"
)

// Every link in the height chain, or none of them work.
//
// The conversation rail scrolls only if it is inside something that refuses to
// grow, which is only true if *its* parent refuses, all the way up to
// .chat-layout. Miss one and the symptom is not "slightly wrong": either the
// page grows and the rail's overflow rule never fires, or an ancestor clips it
// and the rows past the fold cannot be reached at all. Both happened here, one
// after the other.
//
// .chat-pane was the missed one — a plain div between the column and the rail
// that nothing had ever needed to think about.
func TestTheDesktopHeightChainIsComplete(t *testing.T) {
	block, ok := desktopBlock(chatLayoutCSS)
	if !ok {
		t.Fatal("no min-width block, so the whole-screen layout is not scoped to " +
			"a desktop and will be applied to the phone sheet")
	}
	for _, link := range []string{
		".chat-layout",          // the frame: one screen tall
		".chat-side",            // the column
		".chat-side>.chat-pane", // the one that was missing
		".chat-side .chat-rail", // must be allowed to shrink vertically
		".chat-sess-list",       // the part that actually scrolls
	} {
		if !strings.Contains(block, link) {
			t.Errorf("%s is not constrained, so nothing below it can scroll", link)
		}
	}
	// And the list is the only thing that gets a scrollbar in the rail.
	if !strings.Contains(block, ".chat-sess-list{min-height:0;overflow-y:auto}") {
		t.Error("the conversations list does not scroll")
	}
}

// None of it may reach the phone, where the sheet sizes itself and a
// min-height:0 on the same chain collapsed the list to nothing.
func TestTheHeightChainDoesNotReachThePhone(t *testing.T) {
	// Comments out first. This block explains the rules in prose, so a scan of
	// the raw text finds "min-height:0" in the paragraph saying not to put it
	// here — the same false positive the contrast test had.
	css := cssComment.ReplaceAllString(chatLayoutCSS, "")
	before, _, _ := strings.Cut(css, "@media(min-width:761px)")
	for _, leaked := range []string{"min-height:0", "overflow-y:auto", "100vh"} {
		if strings.Contains(before, leaked) {
			t.Errorf("%q is in the base rules, so it applies to the phone sheet too", leaked)
		}
	}
}

// cssComment matches a /* … */ block.
var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// desktopBlock is the @media(min-width:761px) body.
func desktopBlock(css string) (string, bool) {
	_, rest, ok := strings.Cut(css, "@media(min-width:761px){")
	if !ok {
		return "", false
	}
	// To the closing brace of the block: the rules inside have their own, so
	// count them.
	depth := 1
	for i, r := range rest {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[:i], true
			}
		}
	}
	return "", false
}
