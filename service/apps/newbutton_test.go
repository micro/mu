package apps

// Building an app is what this page is for, so its button has to look like a
// button.
//
// It shipped hand-rolled — an anchor with its own background and its own
// color:#fff — and rendered black on black, because the link colour rules in
// the stylesheet outrank a plain class. That is the third time: a.btn carries
// !important for this reason, and connect-cta has a comment about it.
//
// The fix is not another set of colours. It is to use the thing that already
// renders every other page's primary action, so this one cannot drift from
// them or lose to the cascade on its own.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"mu/internal/app"
)

func TestTheNewAppButtonIsTheSharedButton(t *testing.T) {
	want := app.ActionLink("/apps/new", "+ New app")
	if !strings.Contains(want, `class="btn"`) {
		t.Fatalf("ActionLink no longer renders the shared button class: %s", want)
	}
}

// The specific shape that breaks is a *class* on an anchor. A class loses to
// the link-colour rules in the stylesheet, so a white label on a black button
// comes out black on black; an inline style:color wins the cascade and an
// actual <button> never had the problem. That is why the rest of this file's
// dark buttons are fine and the one that broke was `.apps-new`.
//
// Checked against the source, because the cascade cannot be resolved in Go.
func TestTheAppsPageDoesNotRollItsOwnButton(t *testing.T) {
	b, err := os.ReadFile("apps.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if !strings.Contains(src, `app.ActionLink("/apps/new"`) {
		t.Error("the new-app button no longer uses the shared action link")
	}
	// An <a class="..."> whose class sets a dark background: the losing shape.
	classStyled := regexp.MustCompile(`<a class="(?:apps-new|[a-z-]*-cta|[a-z-]*-btn)"`)
	if m := classStyled.FindString(src); m != "" {
		t.Errorf("apps.go styles an action anchor by class (%s) — a class loses to the "+
			"stylesheet's link colour, which is how the label goes black on black. Use app.ActionLink", m)
	}
}
