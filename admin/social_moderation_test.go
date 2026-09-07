package admin

import (
	"mu/internal/flag"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorCanRehideApprovedSocialThroughFlagHandler(t *testing.T) {
	for _, admin := range []bool{false, true} {
		id, account := "ordinary-report", "ordinary-reporter"
		if admin {
			id, account = "operator-report", "operator-reporter"
		}
		cookie := adminSession(t, account, admin)
		if err := flag.Approve("social", id); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/flag", strings.NewReader("type=social&id="+id))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		FlagHandler(w, r)
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if flag.IsHidden("social", id) != admin {
			t.Fatalf("admin=%v hidden=%v", admin, flag.IsHidden("social", id))
		}
	}
}
