package setup

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/settings"
)

func TestApplyProvider_OpenRouter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENROUTER_API_KEY", "")
	prev := settings.Get("OPENROUTER_API_KEY")
	t.Cleanup(func() { settings.Set("OPENROUTER_API_KEY", prev) })
	settings.Set("OPENROUTER_API_KEY", "")

	if err := ApplyProvider("openrouter", "", ""); err == nil {
		t.Fatal("empty key should fail")
	}
	if err := ApplyProvider("openrouter", "sk-or-test", ""); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}
	if got := settings.Get("OPENROUTER_API_KEY"); got != "sk-or-test" {
		t.Fatalf("OPENROUTER_API_KEY = %q", got)
	}
}

func TestApplyProvider_Unknown(t *testing.T) {
	if err := ApplyProvider("nope", "k", ""); err == nil {
		t.Fatal("unknown provider should fail")
	}
}

// The admin account `mu setup` creates has a password its owner chose.
//
// It is created by typing one into the form, and it was recorded without the
// flag that says so — so /account told the first admin of every new instance
// that their account "signs in with Google or a passkey and has no password you
// could type", about the one account on the instance that certainly does have
// one.
//
// Drives applySetup rather than Handler: Handler turns away when an admin
// already exists, and auth loads the real accounts.json at package
// initialisation, before any TestMain here could redirect HOME. Everything
// under test is in applySetup; the guard above it is not what was wrong.
func TestTheSetupAdminHasAPasswordItsOwnerChose(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-not-a-real-key")

	// Distinctive, because auth came up holding whatever accounts this machine
	// has and a common name would collide with a real one.
	const id = "setup_test_first_admin"

	form := url.Values{
		"username": {id},
		"password": {"a password they typed"},
		"provider": {"claude"},
		"key":      {"sk-ant-not-a-real-key"},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	applySetup(rec, req)
	if i := strings.Index(rec.Body.String(), "text-error"); i >= 0 {
		t.Fatalf("setup refused the form: %.200s", rec.Body.String()[i:])
	}

	if _, err := auth.GetAccount(id); err != nil {
		t.Fatalf("setup created no account: %v", err)
	}
	if !auth.HasSecret(id) {
		t.Error("the account created by typing a password is recorded as having none, " +
			"so /account offers to set one and tells them they signed in with Google")
	}
}
