package agent

import (
	"encoding/json"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/news"
	"mu/service/web"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentCreateHTTPScopesAndOwnership(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, spec := range []service.Spec{web.Spec, news.Spec} {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	owner := "agent-create-http"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	old := api.Operations
	api.Operations = PublicOperations()
	defer func() { api.Operations = old }()
	_, write, err := auth.CreateToken(owner, "create", []string{"read", "write", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, read, err := auth.CreateToken(owner, "read", []string{"read", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/agents", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		RosterHandler(w, r)
		return w
	}
	body := `{"name":"Researcher","prompt":"Use sources","services":["web","news"]}`
	if w := request(read, body); w.Code != 403 {
		t.Fatalf("read-only create: %d", w.Code)
	}
	if w := request(write, `{"name":"Researcher","prompt":"Use sources","services":[]}`); w.Code != 400 {
		t.Fatalf("empty scope: %d", w.Code)
	}
	w := request(write, body)
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct{ ID, Agent string }
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Agent == "" || For(owner, created.ID) == nil || For("somebody-else", created.ID) != nil {
		t.Fatal("missing or unscoped agent")
	}
	if For(owner, created.ID).TokenID != "" {
		t.Fatal("unexpected credential created")
	}
}
