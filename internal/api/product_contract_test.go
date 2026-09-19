package api

import (
	"encoding/json"
	"mu/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProductRoutesAndScopeIsolation(t *testing.T) {
	old := Operations
	defer func() { Operations = old }()
	called := false
	Operations = []Operation{{Name: "agent_contract_test", Writes: true, Handle: func(owner string, raw json.RawMessage) (any, error) {
		called = true
		return map[string]string{"owner": owner}, nil
	}}}
	const owner = "product_contract_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Test", Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, product, err := auth.CreateToken(owner, "product", []string{"read", "write", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, services, err := auth.CreateToken(owner, "services", []string{"read", "write", auth.ScopePrefix + "news"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"/agent/api/contract_test", product, 200}, {"/agent/api/contract_test", services, 403}, {"/agent/api/contract_test", "", 401}, {"/api/v1/agent/contract_test", product, 404}, {"/inbox/api/contract_test", product, 404}} {
		called = false
		r := httptest.NewRequest("POST", tc.path, strings.NewReader(`{}`))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		PublicRESTHandler(w, r)
		if w.Code != tc.status || called != (tc.status == 200) {
			t.Fatalf("%s status %d called %v: %s", tc.path, w.Code, called, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+product)
	if err := checkTokenScope(r, "news_list"); err == nil {
		t.Fatal("product token gained service access")
	}
	for _, owner := range []string{"agent", "inbox", "work"} {
		w := httptest.NewRecorder()
		PublicRESTHandler(w, httptest.NewRequest("GET", "/"+owner+"/api", nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		if strings.Contains(w.Body.String(), "agent_contract_test") != (owner == "agent") {
			t.Fatal("discovery crossed owner boundary")
		}
	}
}
