package agent

import (
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestScheduledDetailsAndOwnedEdit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "schedule-detail"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true, Zone: "Europe/London"})
	defer auth.RemoveAccountForTest(owner)
	defer events.DeleteAll(owner)
	sess, _ := auth.CreateSession(owner)
	e, err := events.CreateScheduled(owner, "My reading", time.Now().Add(time.Hour), "", 0, "weekly", "Read about gardening", events.Advance{}, "Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		scheduledHandler(w, r)
		return w
	}
	w := get("/agents?view=scheduled")
	if strings.Contains(w.Body.String(), "<summary>Settings") || strings.Contains(w.Body.String(), `name="clock"`) {
		t.Fatal("list contains editing forms")
	}
	for _, id := range []string{"brief", "checkin", "moment", e.ID} {
		w = get("/agents?view=scheduled&event=" + id)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<form") || strings.Contains(w.Body.String(), "<summary>Settings") {
			t.Fatalf("detail %s: %d", id, w.Code)
		}
	}
	if w = get("/agents?view=scheduled&event=foreign"); w.Code != 404 {
		t.Fatal("unknown event not rejected")
	}
	v := url.Values{"action": {"update-schedule"}, "id": {e.ID}, "title": {"Changed reading"}, "prompt": {"Read about trees"}, "when": {"2030-07-22T10:00"}, "zone": {"Europe/London"}, "repeat": {"daily"}, "prepare": {"yes"}}
	for _, csrf := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/agents?view=scheduled", strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w = httptest.NewRecorder()
		scheduledHandler(w, r)
		want := 403
		if csrf {
			want = 303
		}
		if w.Code != want {
			t.Fatalf("edit: %d %s", w.Code, w.Body.String())
		}
	}
	all := events.List(owner)
	if len(all) != 1 || all[0].ID != e.ID || all[0].Title != "Changed reading" || all[0].Sequence <= e.Sequence || all[0].Zone != "Europe/London" || all[0].Advance.Recipient != "agent" {
		t.Fatalf("invalid edit: %+v", all)
	}
}
