package account

import (
	"context"
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http/httptest"
	"strings"
	"testing"
)

type ScopeProbe struct{}
type ScopeRequest struct{}
type ScopeResponse struct{}

func (*ScopeProbe) List(context.Context, *ScopeRequest, *ScopeResponse) error { return nil }

func TestTokenAllAndSelect(t *testing.T) {
	const owner = "token_scope_ui"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	defer auth.DeleteAccount(owner)
	if err := service.Register(service.Spec{Name: "scopeprobe", Handler: &ScopeProbe{}, Endpoints: map[string]service.Endpoint{"List": {}}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode   string
		names  []string
		status int
		scoped bool
	}{
		{"select", nil, 400, false}, {"select", []string{"unknown"}, 400, false}, {"bad", nil, 400, false},
		{"select", []string{"scopeprobe"}, 200, true}, {"all", []string{"scopeprobe"}, 200, false},
		{"", []string{"scopeprobe"}, 200, true}, {"", nil, 200, false},
	} {
		before := len(auth.ListTokens(owner))
		body, _ := json.Marshal(map[string]any{"name": "test", "scope_mode": tc.mode, "services": tc.names})
		req := httptest.NewRequest("POST", "/token", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handleCreateToken(rec, req, owner)
		if rec.Code != tc.status {
			t.Fatalf("mode=%s status=%d", tc.mode, rec.Code)
		}
		if rec.Code == 400 {
			if len(auth.ListTokens(owner)) != before {
				t.Fatal("invalid selection created token")
			}
			continue
		}
		var result struct {
			ID string `json:"id"`
		}
		json.Unmarshal(rec.Body.Bytes(), &result)
		for _, tok := range auth.ListTokens(owner) {
			if tok.ID == result.ID && (len(tok.Services()) > 0) != tc.scoped {
				t.Fatal("wrong scope")
			}
		}
	}
	rec := httptest.NewRecorder()
	handleTokenPage(rec, httptest.NewRequest("GET", "/token", nil), owner, "fixture")
	page := rec.Body.String()
	for _, want := range []string{`name="scope_mode"`, `value="all">All`, `value="select">Select`, `id="tok-service-list" hidden`, `data-label="Services"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(page, "What may it reach?") {
		t.Fatal("old label")
	}
}
