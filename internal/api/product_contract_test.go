package api

import (
	"mu/internal/auth"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResourceScopeIsolation(t *testing.T) {
	const owner = "resource_contract_owner"
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
	_, readOnly, err := auth.CreateToken(owner, "read", []string{"read", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token, capability string
		write, allowed    bool
	}{{product, "agent", true, true}, {product, "inbox", false, false}, {services, "agent", true, false}, {readOnly, "agent", true, false}, {readOnly, "agent", false, true}, {"", "agent", true, false}} {
		r := httptest.NewRequest("POST", "/agent", nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		if got := AuthorizeProduct(w, r, tc.capability, tc.write); got != tc.allowed {
			t.Fatalf("%s write %v: %d %s", tc.capability, tc.write, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+product)
	if err := checkTokenScope(r, "news_list"); err == nil {
		t.Fatal("product token gained service access")
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("POST", "/agent", nil)
	r.Header.Set("Cookie", "session="+session.Token)
	if AuthorizeProduct(httptest.NewRecorder(), r, "agent", true) {
		t.Fatal("cookie write bypassed CSRF")
	}
	if !AuthorizeProduct(httptest.NewRecorder(), r, "agent", false) {
		t.Fatal("cookie read required CSRF")
	}
}
