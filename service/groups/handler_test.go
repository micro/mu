package groups

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/service"
)

func TestGroupUIAndServiceIsolation(t *testing.T) {
	for _, who := range []string{"groups-ui-owner", "groups-ui-other"} {
		auth.SetAccountForTest(&auth.Account{ID: who})
		defer auth.RemoveAccountForTest(who)
	}
	sess, _ := auth.CreateSession("groups-ui-owner")
	g, err := group.Create("groups-ui-owner", "Family <private>", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { group.Change(g.ID, "groups-ui-owner", "delete", "", "") })
	req := func(method, path, body string) *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	w := httptest.NewRecorder()
	Handler(w, req("GET", "/groups?id="+g.ID, ""))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Open chat") || !strings.Contains(w.Body.String(), "Family &lt;private&gt;") {
		t.Fatal("missing group UI", w.Code)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private page cached")
	}
	w = httptest.NewRecorder()
	Handler(w, req("POST", "/groups", url.Values{"action": {"delete"}, "id": {g.ID}}.Encode()))
	if w.Code != 403 || !group.Member(g.ID, "groups-ui-owner") {
		t.Fatal("CSRF-less delete succeeded")
	}
	var rsp GroupResponse
	if err := (Server{}).Read(service.WithAccount(context.Background(), "groups-ui-other"), &ReadRequest{ID: g.ID}, &rsp); err == nil {
		t.Fatal("service disclosed foreign group")
	}
	w = httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/groups?id="+g.ID, nil))
	if w.Code == 200 {
		t.Fatal("guest read group")
	}
	r := req("POST", "/groups", url.Values{"action": {"invite"}, "id": {g.ID}, "account": {"groups-ui-other"}}.Encode())
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w = httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 303 {
		t.Fatal("could not invite", w.Code, w.Body.String())
	}
	if group.Member(g.ID, "groups-ui-other") {
		t.Fatal("invite auto-joined recipient")
	}
}
