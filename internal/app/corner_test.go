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

func TestAccountIdentityStaysInSidebar(t *testing.T) {
	acc := &auth.Account{ID: "tester"}
	got := headCorner(acc, "")
	if strings.Contains(got, "@tester") || strings.Contains(got, `id="head-me"`) {
		t.Errorf("header duplicates account identity: %q", got)
	}
	if got := navBottom(acc, ""); !strings.Contains(got, "tester") {
		t.Errorf("sidebar lost account identity: %q", got)
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
