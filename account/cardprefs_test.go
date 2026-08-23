package account

// Which cards you see is not a setting at all any more.
//
// There were three editors for it — a panel on /context, a copy on /account,
// and a third list for pinned apps inside the first — and then there was no
// setting, because a composition step in front of the page that is supposed to
// prove the tools work is a step nobody wants. The instance picks the cards.
// These tests hold the ground: no editor comes back, and the one thing on
// /context worth keeping did not go down with it.

import (
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"mu/internal/app"
)

func TestTheAccountPageDoesNotCarryACardPicker(t *testing.T) {
	src, err := os.ReadFile("pages.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// There is no card selection left to edit anywhere: the instance picks the
	// cards and everybody gets the same ones. A page still offering the choice
	// would be offering a setting that does not exist.
	for _, gone := range []string{`name="cards"`, "save_cards", "SetHomeCards", "HomeCardsSeen"} {
		if strings.Contains(body, gone) {
			t.Errorf("the account page still carries card-picker plumbing: %q", gone)
		}
	}
	// The cache card went too. It was here as "what the agent remembers", which
	// is one use of a key-value store and not what the store is; /account is
	// for the account, and a list of arbitrary labels somebody's agent wrote is
	// not a setting.
	if strings.Contains(body, "CacheCard") {
		t.Error("/account is showing the cache again")
	}
}

// A preference edited somewhere other than /account still posts here, because
// this is where the account is written. Answering that by navigating to
// /account would take the reader away from the page they were looking at.
func TestSavingAPreferenceReturnsToThePageItWasEditedOn(t *testing.T) {
	returnTo := func(v string) string {
		r := httptest.NewRequest("POST", "/account", strings.NewReader(url.Values{"return": {v}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ParseForm()
		return app.ReturnTo(r, "/account")
	}

	if got := returnTo("/home"); got != "/home" {
		t.Errorf("saving on /home sent the reader to %q", got)
	}
	if got := returnTo("/apps?tag=timer"); got != "/apps?tag=timer" {
		t.Errorf("pinning on a filtered /apps sent the reader to %q", got)
	}
	if got := returnTo(""); got != "/account" {
		t.Errorf("a form that says nothing should stay put, got %q", got)
	}

	// The field is as forgeable as a query parameter, so it gets the same
	// guard: same-site paths only. "//evil.example" is a URL to a browser.
	for _, hostile := range []string{"https://evil.example", "//evil.example", "javascript:alert(1)"} {
		if got := returnTo(hostile); got != "/account" {
			t.Errorf("return=%q redirected off-site to %q", hostile, got)
		}
	}
}
