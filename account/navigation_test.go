package account

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountDestinations(t *testing.T) {
	const id = "navigation_test_owner"
	if err := auth.Create(&auth.Account{ID: id, Name: "Owner", Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		want   []string
		absent []string
	}{
		{"/account", []string{"Account", "Email", "Password", "Notifications", "Tokens", "Balance", "Usage"}, []string{"Your mail and chat apps", "Developer billing"}},
		{"/account/usage", []string{"Usage", "History", "No transactions yet."}, []string{"API tokens"}},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		w := httptest.NewRecorder()
		if tc.path == "/account/usage" {
			UsageHandler(w, r)
		} else {
			Account(w, r)
		}
		if w.Code != 200 {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		for _, s := range tc.want {
			if !strings.Contains(w.Body.String(), s) {
				t.Errorf("%s missing %s", tc.path, s)
			}
		}
		for _, s := range tc.absent {
			if strings.Contains(w.Body.String(), s) {
				t.Errorf("%s contains %s", tc.path, s)
			}
		}
	}
}
