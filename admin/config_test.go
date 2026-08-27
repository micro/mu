package admin

// The settings page has to answer the question it exists for.
//
// Three of them, and it answered none: what is this set to, where did the value
// come from, and can I change it here. It rendered every value that was set at
// all as "••••••" — the handler fetched the real one and discarded it on the
// next line — so an instance with settings in a .env file, some in the store,
// and the same names in both had no way to tell which was in force.
//
// The worst of it was the environment. settings.Get prefers an environment
// variable permanently, and the page put those in an editable box: type a new
// value, press Save, get "Settings saved", and the value is stored where
// nothing will ever read it.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/settings"
)

// adminRequest is a GET to the settings page as an admin.
func adminRequest(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	const id = "settings_admin"
	if _, err := auth.GetAccount(id); err != nil {
		if err := auth.Create(&auth.Account{ID: id, Name: id, Admin: true}); err != nil {
			// A username the rules refuse is a bug in this test, not a fact about
			// the machine it runs on. Skipping on it is how eighteen tests across
			// this repository quietly stopped running the day usernames became
			// validated — a red suite would have said so on the first push.
			if strings.Contains(err.Error(), "username") {
				t.Fatalf("the test account name is not a valid username: %v", err)
			}
			t.Skipf("cannot create an admin here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	}
	sess, err := auth.InternalSession(id)
	if err != nil {
		t.Skipf("cannot open a session here: %v", err)
	}
	t.Cleanup(func() { auth.EndSession(sess.Token) })

	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	ConfigHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("the settings page answered %d", w.Code)
	}
	return w
}

// TestAValueSetHereIsShown — the whole complaint, in one assertion.
func TestAValueSetHereIsShown(t *testing.T) {
	old := settings.Get("MAIL_DOMAIN")
	settings.Set("MAIL_DOMAIN", "shown.example.com")
	t.Cleanup(func() { settings.Set("MAIL_DOMAIN", old) })

	body := adminRequest(t, "/admin/config").Body.String()
	if !strings.Contains(body, "shown.example.com") {
		t.Error("a setting saved here is not shown on the page that saved it, so " +
			"there is no way to find out what this instance is configured with")
	}
	if !strings.Contains(body, "saved here") {
		t.Error("the page does not say the value came from the store")
	}
}

// TestAnEnvironmentValueCannotBeEditedHere — it wins in settings.Get whatever
// is stored, so an editable box is an invitation to save something nothing
// reads.
func TestAnEnvironmentValueCannotBeEditedHere(t *testing.T) {
	t.Setenv("MU_DOMAIN", "fixed.example.com")

	body := adminRequest(t, "/admin/config").Body.String()
	if !strings.Contains(body, "fixed.example.com") {
		t.Fatal("a value from the environment is not shown at all")
	}
	if !strings.Contains(body, "environment") {
		t.Error("the page does not say where the value came from")
	}

	// The input for it must be readonly and must carry no name, so the POST
	// cannot see it even if somebody edits the DOM.
	i := strings.Index(body, "MU_DOMAIN")
	if i < 0 {
		t.Fatal("MU_DOMAIN is not on the page")
	}
	row := body[i:]
	if end := strings.Index(row, "</div>"); end > 0 {
		row = row[:end]
	}
	if !strings.Contains(row, "readonly") {
		t.Error("a value fixed by the environment is offered in an editable box; " +
			"saving over it stores something settings.Get will never return")
	}
	if strings.Contains(row, `name="MU_DOMAIN"`) {
		t.Error("the environment-fixed input is still named, so a POST would store it")
	}
}

// TestSavingDoesNotOverwriteWhatTheEnvironmentFixed.
func TestSavingDoesNotOverwriteWhatTheEnvironmentFixed(t *testing.T) {
	t.Setenv("MU_DOMAIN", "fixed.example.com")
	stored := settings.All()["MU_DOMAIN"]
	t.Cleanup(func() { settings.Set("MU_DOMAIN", stored) })

	req := httptest.NewRequest("POST", "/admin/config",
		strings.NewReader("MU_DOMAIN=sneaky.example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	const id = "settings_admin_post"
	if _, err := auth.GetAccount(id); err != nil {
		if err := auth.Create(&auth.Account{ID: id, Name: id, Admin: true}); err != nil {
			// A username the rules refuse is a bug in this test, not a fact about
			// the machine it runs on. Skipping on it is how eighteen tests across
			// this repository quietly stopped running the day usernames became
			// validated — a red suite would have said so on the first push.
			if strings.Contains(err.Error(), "username") {
				t.Fatalf("the test account name is not a valid username: %v", err)
			}
			t.Skipf("cannot create an admin here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	}
	sess, err := auth.InternalSession(id)
	if err != nil {
		t.Skipf("cannot open a session here: %v", err)
	}
	t.Cleanup(func() { auth.EndSession(sess.Token) })
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	ConfigHandler(httptest.NewRecorder(), req)

	if got := settings.All()["MU_DOMAIN"]; got == "sneaky.example.com" {
		t.Error("a POST stored a value for a setting the environment fixes — it will " +
			"never be read, and the page will have said it saved")
	}
}

// TestASecretIsNotPrintedInFull — the page is admin-only, not private: it gets
// screen-shared, screenshotted and demonstrated.
func TestASecretIsNotPrintedInFull(t *testing.T) {
	const key = "sk_live_51ABCDEFGHIJKLMNOP"
	old := settings.Get("STRIPE_SECRET_KEY")
	settings.Set("STRIPE_SECRET_KEY", key)
	t.Cleanup(func() { settings.Set("STRIPE_SECRET_KEY", old) })

	body := adminRequest(t, "/admin/config").Body.String()
	if strings.Contains(body, key) {
		t.Error("a secret is printed in full on a page somebody may be sharing")
	}
	// But enough of it to tell which key this is, which is the point of showing
	// anything at all.
	if !strings.Contains(body, "sk_l") {
		t.Error("the secret is masked so completely that an operator cannot tell " +
			"which of their keys it is, which is what •••••• already did")
	}
}

// TestASecretCanBeRevealedDeliberately — masked by default, readable on
// purpose, and through a round trip rather than a script, so it is not sitting
// in the page source of every visit.
func TestASecretCanBeRevealedDeliberately(t *testing.T) {
	const key = "sk_live_51ABCDEFGHIJKLMNOP"
	old := settings.Get("STRIPE_SECRET_KEY")
	settings.Set("STRIPE_SECRET_KEY", key)
	t.Cleanup(func() { settings.Set("STRIPE_SECRET_KEY", old) })

	body := adminRequest(t, "/admin/config?reveal=STRIPE_SECRET_KEY").Body.String()
	if !strings.Contains(body, key) {
		t.Error("asking to see a secret does not show it, so the only way to check " +
			"which key is configured is to read the disk")
	}
}

// TestThePageSaysHowManyComeFromWhere — the summary, because "which is which"
// is asked about the whole instance and a per-row badge answers it one row at a
// time.
func TestThePageSaysHowManyComeFromWhere(t *testing.T) {
	body := adminRequest(t, "/admin/config").Body.String()
	for _, want := range []string{"from the environment", "saved here", "not set"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not summarise %q", want)
		}
	}
}
