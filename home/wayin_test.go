package home

// One way in, in one place.
//
// Signed out, this product had four: a Log in in the shell's corner, a Sign up
// and a Log in beside the date on /home, and a Login in the rail. The corner is
// the one that is always there and always in the same place, which is the whole
// argument for having a corner — see app.headCorner and the note on
// topRight. The pair beside the date was written when this screen was the
// landing and was the only place to sign in from; it outlived that by a month
// and sat a few centimetres under a link doing the same thing in a different
// style.

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Nothing in the body of the signed-out home offers a way in.
func TestSignedOutHomeDoesNotOfferASecondWayIn(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/home", nil))
	body := rec.Body.String()

	if strings.Contains(body, "home-date-actions") {
		t.Error("the Sign up / Log in pair is back beside the date, under the corner that already says it")
	}
	// The corner itself is the shell's and is expected — this is about the
	// page's own content, so it is the count that matters rather than the
	// presence.
	if n := strings.Count(body, `href="/signup"`); n > 0 {
		t.Errorf("the signed-out home offers Sign up %d times in the page; the login page offers it", n)
	}
}

// The corner is in the corner.
//
// The landing's login link is absolutely positioned, and what it is positioned
// against decides where it lands. It was anchored to the header row — a centred
// 760px column — so on a wide desktop it sat at the right edge of that column,
// in the middle of the empty half of the page, rather than in the corner of the
// screen. It looked right only while that row also held a wordmark, and the
// wordmark moved into the body when the landing became one centred stack.
//
// Asserted on the stylesheet because the placement is entirely CSS: the markup
// was correct throughout.
func TestTheLandingCornerIsAnchoredToThePage(t *testing.T) {
	b, err := os.ReadFile("../internal/app/index.go")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)

	// The header row must not be a containing block, or the corner resolves
	// against it again.
	head := section(css, ".index-head{")
	if strings.Contains(head, "position:relative") {
		t.Error("the header row is positioned again, so the corner is back at the edge of a 760px column")
	}

	corner := section(css, ".login-link{")
	if !strings.Contains(corner, "position:absolute") {
		t.Fatalf("the corner is no longer absolutely positioned: %q", corner)
	}
	// Not right:0 — an absolute box is placed against the containing block's
	// padding box, so right:0 is the window's own edge with the page's gutter
	// passing behind the link.
	if strings.Contains(corner, "right:0") {
		t.Error("the corner is flush to the window edge; it should match the page's 20px gutter")
	}
	// And not centred in a row whose height depends on whether a wordmark is
	// in it: the landing has none, and centring collapsed it to the top edge.
	if strings.Contains(corner, "top:50%") {
		t.Error("the corner centres in the header row again, which has no height on the landing")
	}
}

// section returns the declaration block that starts with the given selector.
func section(css, selector string) string {
	i := strings.Index(css, selector)
	if i < 0 {
		return ""
	}
	rest := css[i:]
	if j := strings.Index(rest, "}"); j >= 0 {
		return rest[:j]
	}
	return rest
}
