package app

// Which cards you watch is one setting, and it is edited in one place.
//
// There were three editors for it: the panel on /context, next to the cards it
// governs; a second copy on /account; and, for pinned apps, a third list inside
// the first. /account's copy was the one that could not show you what a tick
// would do, because the cards are not on that page — and it had already drifted,
// rendering the instance's order rather than the reader's and offering no way
// to reorder at all.

import (
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestTheAccountPageDoesNotCarryASecondCardPicker(t *testing.T) {
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	i := strings.Index(body, "homeCardsCard :=")
	if i < 0 {
		t.Fatal("the account page no longer has a Home Screen card at all")
	}
	j := strings.Index(body[i:], "\n\t// Chat channel link card")
	if j < 0 {
		t.Fatal("could not find the end of the Home Screen card")
	}
	card := body[i : i+j]

	if strings.Contains(card, `name="cards"`) {
		t.Error("/account is editing the card selection again; it belongs on /context, " +
			"beside the cards, where a tick shows you what it did")
	}
	if !strings.Contains(card, "/context?cards=1") {
		t.Error("/account no longer points at the one place the setting is edited")
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
		return prefsReturnTo(r)
	}

	if got := returnTo("/context"); got != "/context" {
		t.Errorf("saving on /context sent the reader to %q", got)
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

func TestPinningAndUnpinningAnAppByName(t *testing.T) {
	// By name rather than by restating the whole set, so a stale tab cannot
	// unpin what another one just pinned.
	have := addWidget(nil, "pomodoro")
	have = addWidget(have, "packing")
	have = addWidget(have, "pomodoro") // already there
	if strings.Join(have, ",") != "pomodoro,packing" {
		t.Errorf("pinning gave %v", have)
	}

	have = removeWidget(have, "pomodoro")
	if strings.Join(have, ",") != "packing" {
		t.Errorf("unpinning gave %v", have)
	}
	if got := removeWidget(have, "never-pinned"); strings.Join(got, ",") != "packing" {
		t.Errorf("unpinning something that was not pinned gave %v", got)
	}
}
