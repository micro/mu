package home

// What closing the sidebar does to a page.
//
// Three properties, all of them CSS, all of them invisible to every other test
// here because the markup does not change: what the browser draws does.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func styles(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Closing the sidebar gives a page more room, never less.
//
// It gave less. Collapsed capped #content at 1080px while open it is 1400, or
// 1700 on Home — so on any screen past about 1300 the page narrowed as the rail
// went away. Measured at 1728: the news card was 692px wide with the rail open
// and 272 with it closed, because the 320px right-hand track kept its width and
// the whole loss landed on the column beside it.
//
// The rule that did it is the thing to keep out. A max-width on the collapsed
// content box can only ever be a number smaller than the open one, which is the
// bug however it is spelled.
func TestClosingTheSidebarDoesNotNarrowThePage(t *testing.T) {
	css := styles(t)
	rule := regexp.MustCompile(`body\.nav-collapsed[^{}]*#content\s*\{[^}]*\}`)
	for _, block := range rule.FindAllString(css, -1) {
		if strings.Contains(block, "max-width") {
			t.Errorf("the collapsed content box is capped again, which makes a "+
				"page narrower with the rail closed than open:\n%s", block)
		}
	}
}

// And a column of prose centres when there is no rail to align to.
//
// This is what the cap was for, and it is the only thing that actually needed
// fixing: a page that sets .page-col has said its content is one 760px column,
// and with the rail gone that column sat against the left of a wide window.
// Nothing else is narrowed to solve it.
func TestProseCentresWhenTheRailIsGone(t *testing.T) {
	css := styles(t)
	if !regexp.MustCompile(`body\.nav-collapsed\s+\.page-col\s*\{[^}]*margin-inline:\s*auto`).MatchString(css) {
		t.Error("a one-column page does not centre with the sidebar closed, so it " +
			"is left hanging off the edge it was aligned to")
	}
}

// The corner says who you are only when the rail is not already saying it.
//
// The rail's foot carries "Signed in as @you" with Account under it, and the
// corner carries @you linking to the same page. Collapsed, the rail is not on
// screen and the corner is the only answer — which is why it exists. Open, both
// were on screen at once, in two styles, one above the other.
func TestTheCornerDoesNotRepeatTheRail(t *testing.T) {
	css := styles(t)
	if !regexp.MustCompile(`body:not\(\.nav-collapsed\)\s+#head-me\s*\{[^}]*display:\s*none`).MatchString(css) {
		t.Error("the corner names the account while the rail is open and naming it too")
	}
	// And only on a desktop, where the rail is a column beside the corner.
	// Below 901px it is an overlay and there is nothing on screen to repeat, so
	// a rule that was not scoped to that width would take the name away on a
	// phone as well — where the rail is a tap away rather than a glance.
	desktop := strings.Index(css, "min-width: 901px")
	scoped := strings.Index(css, "body:not(.nav-collapsed) #head-me")
	if desktop < 0 || scoped < desktop {
		t.Error("the corner's name is hidden outside the desktop rule, where the rail " +
			"is an overlay and repeats nothing")
	}
}

// News on Home is headlines rather than articles.
//
// Eight items with a category, a title, the feed's description and a source
// line came to 882px, against 469 for the next biggest card and 164 for the
// smallest: one card was half the page on a screen of glances. The description
// is a third telling of the same thing — the brief above it is the synthesis
// and /news is the article — so it goes, and the items lay out in columns when
// the card is wide enough for them.
//
// Not a time window. Cutting to the last few hours empties the card overnight,
// which is exactly when there is most to catch up on.
func TestTheNewsCardIsAGlance(t *testing.T) {
	css := styles(t)
	if !strings.Contains(css, "#home #news .headline .description { display: none; }") {
		t.Error("the news card is showing article descriptions again, which is what " +
			"made it twice the height of every other card")
	}
	grid := regexp.MustCompile(`#home #news \.section\s*\{[^}]*\}`).FindString(css)
	if !strings.Contains(grid, "auto-fill") {
		t.Errorf("the headlines do not lay out in columns: %q", grid)
	}
	// auto-fill and not a media query, because what decides the number of
	// columns is the card's width, and that depends on the rail as much as on
	// the window.
	if strings.Contains(grid, "@media") {
		t.Error("the headline columns are keyed to the window rather than to the card")
	}
}
