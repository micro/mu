package admin

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUserDetailsRemainAdminOnly(t *testing.T) {
	for _, a := range []*auth.Account{{ID: "details-admin", Admin: true, Approved: true}, {ID: "details-user", Approved: true, Email: "verified@example.test", EmailVerified: true, EmailVerifiedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}} {
		auth.SetAccountForTest(a)
		defer auth.RemoveAccountForTest(a.ID)
	}
	for _, id := range []string{"details-admin", "details-user"} {
		s, _ := auth.CreateSession(id)
		r := httptest.NewRequest("GET", "/admin/users", nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: s.Token})
		w := httptest.NewRecorder()
		UsersHandler(w, r)
		body := w.Body.String()
		if id == "details-user" {
			if w.Code != 403 || strings.Contains(body, "verified@example.test") {
				t.Fatal("user details exposed")
			}
			continue
		}
		if w.Code != 200 || !strings.Contains(body, "verified@example.test · Verified on 2026-10-01") || !strings.Contains(body, `class="collection-item collection-disclosure"`) {
			t.Fatal("user details missing")
		}
		if dir := os.Getenv("ADMIN_PREVIEW_DIR"); dir != "" {
			os.WriteFile(dir+"/users.html", w.Body.Bytes(), 0600)
		}
	}
}
