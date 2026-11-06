package agent

import (
	"mu/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientCatalogScopesAndPresentation(t *testing.T) {
	const owner = "client_catalog_owner"
	const other = "client_catalog_other"
	for _, id := range []string{owner, other} {
		if err := auth.Create(&auth.Account{ID: id, Admin: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := CreateAgent(owner, "Malten", Hosted, "PRIVATE INSTRUCTIONS", "For reflection", nil, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateAgent(other, "PrivateOther", Hosted, "SECRET OTHER", "Hidden description", nil, false); err != nil {
		t.Fatal(err)
	}
	_, allowed, err := auth.CreateToken(owner, "client", []string{"read", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, denied, err := auth.CreateToken(owner, "service", []string{"read", auth.ScopePrefix + "news"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{allowed, denied, ""} {
		r := httptest.NewRequest("GET", "/agent?view=agents", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		if token != allowed {
			if w.Code == 200 {
				t.Fatal("unauthorized catalog")
			}
			continue
		}
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, `"description":"For reflection"`) || !strings.Contains(body, `"slug":"malten"`) {
			t.Fatalf("catalog: %d %s", w.Code, body)
		}
		for _, secret := range []string{"PRIVATE INSTRUCTIONS", "SECRET OTHER", "PrivateOther", "Hidden description"} {
			if strings.Contains(body, secret) {
				t.Fatal("catalog exposed private context")
			}
		}
	}
}
