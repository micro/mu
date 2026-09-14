package account

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPendingAccountsCannotCreateCredentials(t *testing.T) {
	// Reserve the bootstrap operator before creating an ordinary pending account.
	if err := auth.Create(&auth.Account{ID: "credential_operator", Admin: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount("credential_operator") })
	const owner = "credential_pending"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(owner) })
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body string }{
		{"/token", "name=test"},
		{"/token?delete_client=anything", "name=test"},
		{"/token?create_client=1", "_method=DELETE&client_name=test"},
	} {
		r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		TokenHandler(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: status %d", tc.path, w.Code)
		}
	}
	if len(auth.ListTokens(owner)) != 0 || len(auth.OAuthClientsFor(owner)) != 0 {
		t.Fatal("pending account received credentials")
	}
}
