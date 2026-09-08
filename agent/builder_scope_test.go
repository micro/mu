package agent

import (
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/news"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBuilderAllSelectAndRemovedGenerator(t *testing.T) {
	const who = "builder_scope"
	if err := auth.Create(&auth.Account{ID: who, Admin: true, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(news.Spec); err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "session", Value: sess.Token}
	req := httptest.NewRequest("GET", "/agent/new", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	NewAgentHandler(rec, req)
	page := rec.Body.String()
	for _, want := range []string{`id="b-scope"`, `value="all">All`, `value="select">Select`, `id="b-service-list" hidden`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"Describe it", "bGen()", "b-genbtn", "What may it reach?"} {
		if strings.Contains(page, bad) {
			t.Errorf("old builder feature %q", bad)
		}
	}
	for _, tc := range []struct {
		action, mode, tools string
		status              int
	}{{"generate", "all", "", 400}, {"save", "select", "", 400}, {"save", "select", "unknown", 400}, {"save", "select", "news", 200}, {"save", "all", "news", 200}} {
		before := len(Agents(who))
		form := url.Values{"action": {tc.action}, "scope_mode": {tc.mode}, "name": {"Example" + tc.mode}, "prompt": {"Be useful"}, "tools": {tc.tools}}
		req := httptest.NewRequest("POST", "/agents/data", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		AgentsHandler(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, rec.Code, rec.Body.String())
		}
		if rec.Code == 400 && len(Agents(who)) != before {
			t.Fatal("invalid request created agent")
		}
	}
	for _, a := range Agents(who) {
		if (a.Name == "Exampleselect") != (len(a.Services) > 0) {
			t.Fatalf("incorrect services: %+v", a.Services)
		}
		req := httptest.NewRequest("GET", "/agent/new?id="+a.ID, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		NewAgentHandler(rec, req)
		if a.Name == "Exampleselect" && !strings.Contains(rec.Body.String(), `value="select" selected`) {
			t.Fatal("edit lost selection")
		}
		RemoveAgent(who, a.ID)
	}
}
