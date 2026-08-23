package browser

// What this refuses, and what it says when it cannot work.
//
// The browser itself is not exercised — that needs Chromium, and an instance
// without one is the case most instances are in. What is worth holding is
// everything either side of it: the URLs it will not open, the answer when
// there is nothing to open them with, and that a picture's address cannot be
// turned into a way into the blob store.

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// An agent picks a URL having read text a stranger wrote. 127.0.0.1 is this
// instance's own surface and 169.254.169.254 hands out cloud credentials to
// anything that asks; both are ordinary URLs to a browser.
func TestItWillNotOpenTheMachineItRunsOn(t *testing.T) {
	t.Setenv("BROWSER_URL", "ws://browser:9222")

	for _, bad := range []string{
		"http://127.0.0.1:8080/admin",
		"http://localhost/admin",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/",
		"http://192.168.1.1/",
		"http://[::1]/",
		"http://metadata.google.internal/",
		"file:///etc/passwd",
		"ftp://example.com/",
	} {
		if _, err := checked(bad); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}

	// And an ordinary page is not refused, or the guard is a wall.
	if _, err := checked("https://example.com/page"); err != nil {
		t.Errorf("a public page was refused: %v", err)
	}
	// A bare host is a host. An agent that types example.com means https.
	got, err := checked("example.com")
	if err != nil || got != "https://example.com" {
		t.Errorf("a bare host became %q (%v)", got, err)
	}
}

// An instance with no browser says so, in words an operator can act on, rather
// than failing somewhere inside a websocket.
// An instance with no browser says so, in words an operator can act on, rather
// than failing somewhere inside a websocket.
//
// Skipped where a browser is actually installed, which is the case this test
// cannot create — and the case that needs no configuration at all, which is the
// point of found().
func TestWithNoBrowserItSaysSo(t *testing.T) {
	t.Setenv("BROWSER_URL", "")
	t.Setenv("CHROME_PATH", "")
	if found() != "" {
		t.Skip("a browser is installed here, so there is no unconfigured case to test")
	}

	if Configured() {
		t.Fatal("an instance with no browser anywhere reports one")
	}
	_, err := checked("https://example.com")
	if err == nil {
		t.Fatal("a page was accepted with no browser to open it in")
	}
	for _, want := range []string{"BROWSER_URL", "CHROME_PATH", "install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %s: %v", want, err)
		}
	}

	// The page says the same thing rather than offering a box that cannot work.
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/browser", nil))
	if body := w.Body.String(); !strings.Contains(body, "could not find one") {
		t.Errorf("the page does not say the browser is missing:\n%s", body)
	}
}

// A machine with a browser on it needs no settings. Configuration that exists
// only because nobody looked is configuration worth deleting.
func TestAnInstalledBrowserNeedsNoSettings(t *testing.T) {
	t.Setenv("BROWSER_URL", "")
	t.Setenv("CHROME_PATH", "")
	if found() == "" {
		t.Skip("no browser installed here")
	}
	if !Configured() {
		t.Errorf("a browser at %s was not used", found())
	}
	if binary() != found() {
		t.Errorf("binary() is %q, want the one found at %q", binary(), found())
	}
}

// And a named one wins, because naming it is how you choose between two.
func TestANamedBrowserWinsOverTheFoundOne(t *testing.T) {
	t.Setenv("CHROME_PATH", "/opt/my/chrome")
	if got := binary(); got != "/opt/my/chrome" {
		t.Errorf("CHROME_PATH was ignored: %q", got)
	}
}

// The refusal is checked before the browser is, so an instance with one still
// will not open its own admin page.
func TestTheGuardRunsBeforeTheBrowserCheck(t *testing.T) {
	t.Setenv("BROWSER_URL", "")
	t.Setenv("CHROME_PATH", "")

	_, err := checked("http://127.0.0.1/admin")
	if err == nil || strings.Contains(err.Error(), "BROWSER_URL") {
		t.Errorf("a private URL was answered with a configuration message: %v", err)
	}
}

// A picture's address names a picture and nothing else. It is rebuilt from the
// path rather than taken from it, so there is no traversal to think about.
func TestAPictureAddressIsNotAWayIntoTheStore(t *testing.T) {
	for _, bad := range []string{
		"../../../secrets", "a/b", "..", "", "zzzz", strings.Repeat("a", 31),
		strings.Repeat("a", 33), "ABCDEF0123456789abcdef0123456789",
	} {
		if hexID(bad) {
			t.Errorf("%q was accepted as a picture name", bad)
		}
	}
	if !hexID("0123456789abcdef0123456789abcdef") {
		t.Error("a real picture name was refused")
	}

	// And the key it builds always sits under the shot prefix.
	key := shotKey("https://example.com", true)
	if !strings.HasPrefix(key, shotPrefix) || !strings.HasSuffix(key, ".png") {
		t.Errorf("a picture is stored at %q", key)
	}
	if shotKey("https://example.com", true) == shotKey("https://example.com", false) {
		t.Error("a full-page shot and a screenful share a key, so one overwrites the other")
	}
}

// innerText comes back with runs of blank lines wherever a layout element held
// no text, and there are a lot of them. Left in, they are a third of what a
// model is charged to read.
func TestBlankRunsAreSqueezedOut(t *testing.T) {
	got := tidy("Title\n\n\n\n\nBody text   \n\n\n\nEnd\n\n\n")
	if got != "Title\n\nBody text\n\nEnd" {
		t.Errorf("tidy gave %q", got)
	}
}
