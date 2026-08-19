package app

// Navigating has to show you the page you asked for.
//
// On a phone the sidebar is an overlay sitting on top of the content. Tapping
// a nav item swapped the new page in behind it and left it there: the page had
// arrived and you could not see it, and the only way out was to find the
// overlay's edge and tap that.
//
// Nothing was wrong before soft navigation, which is what makes it the kind of
// bug that gets shipped. A real page load closed the menu as a side effect of
// throwing the whole document away, so no code ever had to say "close the
// menu" — and when the reload went, the thing it had been doing for free went
// with it, silently, on the one viewport where it matters.
//
// Checked against the rendered shell rather than the source string, because
// the shell is what a browser gets.

import (
	"strings"
	"testing"
)

func shell(t *testing.T) string {
	t.Helper()
	out := RenderHTML("Test", "a page", "<p>hello</p>", nil)
	if !strings.Contains(out, "menu-toggle") {
		t.Fatal("the rendered page has no menu button, so this test is looking at the wrong thing")
	}
	return out
}

// TestASoftNavigationClosesTheMenu.
func TestASoftNavigationClosesTheMenu(t *testing.T) {
	out := shell(t)
	if !strings.Contains(out, "function closeMenu()") {
		t.Fatal("nothing closes the menu")
	}
	// The navigation function has to do it. Everything that changes the page
	// goes through go() — a link, and the back button.
	i := strings.Index(out, "function go(url, push, restoreY) {")
	if i < 0 {
		t.Fatal("the soft navigation function is not there any more; this test needs rewriting")
	}
	body := out[i:]
	if end := strings.Index(body, "\n        }"); end > 0 {
		body = body[:end]
	}
	if !strings.Contains(body, "closeMenu()") {
		t.Error("a soft navigation does not close the menu, so on a phone the new page " +
			"is swapped in behind a sidebar that is still covering it")
	}
}

// TestTappingTheSidebarClosesIt — the other half. Some links never reach go():
// a hash on the page you are already on is the browser's business, and a link
// to where you already are returns early. Both leave the menu over the content.
func TestTappingTheSidebarClosesIt(t *testing.T) {
	out := shell(t)
	if !strings.Contains(out, "closest('#nav-container')") {
		t.Error("a tap in the sidebar only closes it when it turns into a navigation, " +
			"so tapping the page you are already on leaves the menu open over it")
	}
}

// TestTheMenuIsOnlyAnOverlayOnASmallScreen — the fix must not reach the
// desktop sidebar, which is permanent and whose collapsed state is a
// deliberate choice that is remembered.
func TestTheMenuIsOnlyAnOverlayOnASmallScreen(t *testing.T) {
	out := shell(t)
	if !strings.Contains(out, "min-width: 901px") {
		t.Error("the menu button no longer distinguishes a phone from a desktop, so " +
			"closing the menu on navigation would collapse a sidebar somebody left open")
	}
	if strings.Contains(out, "closeMenu") &&
		strings.Contains(out, "closeMenu() { document.body.classList.remove('nav-collapsed')") {
		t.Error("closing the menu also un-collapses the desktop sidebar, which is a " +
			"setting rather than a state")
	}
}
