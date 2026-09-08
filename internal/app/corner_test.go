package app

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestGuestsSignInThroughTheSidebar(t *testing.T) {
	for _, inviteOnly := range []string{"false", "true"} {
		t.Setenv("INVITE_ONLY", inviteOnly)
		if got := headCorner(nil, "/news"); got != "" {
			t.Errorf("guest header duplicates account links: %q", got)
		}
		if got := navMain(nil); got != "" {
			t.Errorf("guest sidebar exposes signed-in destinations: %q", got)
		}
		if got := navBottom(nil, ""); !strings.Contains(got, `href="/login"`) {
			t.Errorf("guest sidebar has no way to log in: %q", got)
		}
	}
}

// Account identity stays in the sidebar even when the sidebar is collapsed.
func TestAccountIdentityStaysInTheSidebar(t *testing.T) {
	account := &auth.Account{ID: "tester"}
	if got := headCorner(account, ""); strings.Contains(got, "@tester") || strings.Contains(got, `id="head-me"`) {
		t.Fatalf("duplicate header identity: %s", got)
	}
	if got := navBottom(account, ""); !strings.Contains(got, "@tester") {
		t.Fatalf("sidebar lost identity: %s", got)
	}
}

// A username is somebody's own text and lands in markup.
func TestTheNameInTheCornerIsEscaped(t *testing.T) {
	got := headCorner(&auth.Account{ID: `<script>x</script>`}, "")
	if strings.Contains(got, "<script>") {
		t.Errorf("an account id went into the corner as markup: %q", got)
	}
}

func TestSidebarLoginReturnsToTheCurrentPage(t *testing.T) {
	got := navBottom(nil, "/archive?q=go+micro")
	if !strings.Contains(got, "redirect=%2Farchive%3Fq%3Dgo%2Bmicro") {
		t.Errorf("sidebar login loses the current page: %q", got)
	}
	if strings.Contains(navBottom(nil, "/"), "redirect=") {
		t.Error("landing login should proceed to Home")
	}
}
