package admin

// Granting credits from the page where the accounts are.
//
// It existed only as a console command — `credit <user> <amount>` — which is to
// say it existed for somebody who already knew. An operator looking at a list of
// users, wanting to comp one of them, found buttons for banning and approving,
// nothing about money, and reasonably concluded the product could not do it.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/account"
	"mu/internal/auth"
)

func adminSession(t *testing.T, id string, admin bool) *http.Cookie {
	t.Helper()
	auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}) //nolint:errcheck
	acc, err := auth.GetAccount(id)
	if err != nil {
		// A username the rules refuse is a bug in this test, not a fact about
		// the machine it runs on. Skipping on it is how eighteen tests across
		// this repository quietly stopped running the day usernames became
		// validated — a red suite would have said so on the first push.
		if strings.Contains(err.Error(), "username") {
			t.Fatalf("the test account name is not a valid username: %v", err)
		}
		t.Skipf("cannot create an account here: %v", err)
	}
	acc.Admin = admin
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck

	sess, err := auth.CreateSession(id)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

func grant(t *testing.T, cookie *http.Cookie, to, amount string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"action": {"credit"}, "user_id": {to}, "amount": {amount}}
	r := httptest.NewRequest("POST", "/admin/users", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	rr := httptest.NewRecorder()
	UsersHandler(rr, r)
	return rr
}

func TestAnAdminCanCreditAnAccountFromTheUsersPage(t *testing.T) {
	admin := adminSession(t, "admin-credit-giver", true)
	adminSession(t, "admin-credit-taker", false)

	before := account.Balance("admin-credit-taker")
	grant(t, admin, "admin-credit-taker", "500")

	if got := account.Balance("admin-credit-taker"); got != before+500 {
		t.Errorf("balance is %d, want %d — the grant did not land", got, before+500)
	}
}

// Nothing silly gets banked. AddCredits creates a wallet for whatever string it
// is handed, so a bad amount must be refused before it reaches one.
func TestAJunkAmountIsRefusedRatherThanBanked(t *testing.T) {
	admin := adminSession(t, "admin-credit-junk", true)
	adminSession(t, "admin-credit-target", false)

	before := account.Balance("admin-credit-target")
	for _, amount := range []string{"", "0", "-100", "lots"} {
		grant(t, admin, "admin-credit-target", amount)
		if got := account.Balance("admin-credit-target"); got != before {
			t.Fatalf("%q changed the balance to %d", amount, got)
		}
	}
}

// And it is admin-only, like everything else on that page.
func TestANonAdminCannotGrantCredits(t *testing.T) {
	notAdmin := adminSession(t, "admin-credit-nobody", false)
	adminSession(t, "admin-credit-victim", false)

	before := account.Balance("admin-credit-victim")
	rr := grant(t, notAdmin, "admin-credit-victim", "1000")

	if rr.Code == http.StatusSeeOther {
		t.Error("a non-admin's grant was accepted")
	}
	if got := account.Balance("admin-credit-victim"); got != before {
		t.Errorf("a non-admin moved the balance to %d", got)
	}
}
