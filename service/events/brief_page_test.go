package events

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBriefScheduleFormAndAuthorization(t *testing.T) {
	owner := "schedule_brief"
	auth.Create(&auth.Account{ID: owner, Name: owner, Zone: "Europe/London"})
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteAll(owner)
	cookie := &http.Cookie{Name: "session", Value: sess.Token}
	get := httptest.NewRequest("GET", "/events", nil)
	get.AddCookie(cookie)
	token := auth.CSRFToken(get)
	for _, tc := range []struct {
		signed bool
		token  string
		status int
	}{{false, "", 401}, {true, "", 403}, {true, "bad", 403}, {true, token, 303}} {
		form := url.Values{"action": {"brief-schedule"}, "clock": {"20:00"}, "zone": {"Europe/London"}, "period": {"evening"}, "repeat": {"daily"}, "state": {"active"}, "_csrf": {tc.token}, "owner": {"someone-else"}}
		req := httptest.NewRequest("POST", "/events", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if tc.signed {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		Handler(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("status %d want %d: %s", rec.Code, tc.status, rec.Body.String())
		}
	}
	if Brief(owner) == nil || Brief("someone-else") != nil {
		t.Fatal("wrong owner")
	}
	got := briefScheduleHTML(owner, token)
	for _, s := range []string{"<summary>Manage</summary>", "Daily at 20:00", `value="paused"`, token} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q", s)
		}
	}
	if briefScheduleHTML("") != "" {
		t.Fatal("guest schedule exposed")
	}
}
