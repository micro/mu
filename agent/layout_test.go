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
		".chat-sess-scroll",     // the part that actually scrolls
	} {
		if !strings.Contains(block, link) {
			t.Errorf("%s is not constrained, so nothing below it can scroll", link)
		}
	}
	if !strings.Contains(block, ".chat-sess-scroll{flex:1 1 auto;min-height:0;overflow-y:auto}") {
		t.Error("the rail does not scroll")
	}
}

// One scroll region in the rail, not one per list.
//
// The rule was on .chat-sess-list, written when the rail held a single list. It
// then grew a second — what the agent answered in the inbox — and two
// auto-height scrollers in one fixed-height column each size to their own
// content, so together they overrun the rail and the second is painted over the
// bottom of the first. Reported exactly that way: a section overlapping the
// conversations, on a live instance.
//
// The scroll belongs to the column. Asserted rather than commented, because the
// next section added to this rail will be added by somebody who was not here.
func TestOnlyTheRailScrollsAndNotEachListInIt(t *testing.T) {
	css := cssComment.ReplaceAllString(chatLayoutCSS, "")
	for _, rule := range []string{
		".chat-sess-list{min-height:0;overflow-y:auto}",
		".chat-sess-list{flex-direction:column;overflow:visible;flex-wrap:nowrap;max-height:52vh;overflow-y:auto}",
	} {
		if strings.Contains(css, rule) {
			t.Errorf("%s makes every list its own scroll region; the rail has more "+
				"than one list, and they overlap", rule)
		}
	}
	// Whatever else it says, the list itself must not carry a height cap or an
	// overflow of its own.
	for _, decl := range listDecls(css) {
		if strings.Contains(decl, "overflow-y:auto") || strings.Contains(decl, "max-height") {
			t.Errorf(".chat-sess-list declares %q — that belongs on .chat-sess-scroll", decl)
		}
	}
}

// listDecls returns the body of every `.chat-sess-list{…}` rule.
func listDecls(css string) []string {
	var out []string
	for _, m := range listRule.FindAllStringSubmatch(css, -1) {
		out = append(out, m[1])
	}
	return out
}

var listRule = regexp.MustCompile(`\.chat-sess-list\{([^}]*)\}`)

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

// A custom property referenced only in its own fallback is not a setting, it is
// a constant with extra steps.
//
// .chat-layout is sized calc(100vh - var(--chat-chrome, 120px)) and nothing on
// the instance ever set --chat-chrome, so every page used 120px while the
// chrome above the layout measures 192px. Measured on micro.mu at 1440x900: the
// rail's bottom edge at 972px, seventy-two pixels below the window, clipped by
// .chat-side{overflow:hidden}.
//
// It stayed hidden for as long as the rail ended in a scrolling list, because
// what fell off the bottom was the empty part of a scroll region. The moment a
// second section was pinned below it, the overflow became the section — which
// is how this was reported: something overlapping the bottom of the
// conversations, on somebody's own instance.
//
// So the variable has to be written by something that can see the rendered
// page. This test only checks that something does: a page that reads the
// property and never assigns it is back to a hard-coded 120.
func TestTheChromeAboveTheChatIsMeasuredAndNotAssumed(t *testing.T) {
	if !strings.Contains(chatLayoutCSS, "var(--chat-chrome") {
		t.Fatal("the layout no longer reads --chat-chrome; this test is stale")
	}
	if !strings.Contains(chatPageJS, "setProperty('--chat-chrome'") {
		t.Error("--chat-chrome is read by the stylesheet and never set, so the " +
			"fallback is the only value there has ever been")
	}
	// And re-measured, because the number changes with the window: the nav
	// wraps, a banner appears, a font lands and moves everything down.
	if !strings.Contains(chatPageJS, "addEventListener('resize',muChatChrome)") {
		t.Error("--chat-chrome is measured once and never again, so it is wrong " +
			"as soon as the window changes")
	}
}
