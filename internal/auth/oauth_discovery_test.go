package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"mu/internal/service"
)

func TestOAuthDiscoveryScopes(t *testing.T) {
	var previous []string
	for _, handler := range []http.HandlerFunc{OAuthMetadataHandler, OAuthResourceHandler} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", "https://runtime.example/", nil))
		var metadata struct {
			Scopes []string `json:"scopes_supported"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &metadata); err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"read", "write", "api:agent", "api:inbox", "api:work", "api:home", "api:services"} {
			if !slices.Contains(metadata.Scopes, required) {
				t.Errorf("missing scope %s", required)
			}
		}
		for _, spec := range service.Specs() {
			if !slices.Contains(metadata.Scopes, ScopePrefix+spec.Name) {
				t.Errorf("missing service %s", spec.Name)
			}
		}
		for _, scope := range metadata.Scopes {
			if strings.HasPrefix(scope, "protocol:") {
				t.Errorf("advertised scope unavailable through OAuth consent: %s", scope)
			}
		}
		if previous != nil && !slices.Equal(previous, metadata.Scopes) {
			t.Fatal("authorization and resource metadata disagree")
		}
		previous = metadata.Scopes
	}
	// Discovery must not conflate permission to ask an agent with permission to
	// call its underlying services directly (or vice versa).
	agent := &Token{Permissions: []string{"read", "write", "api:agent"}}
	if agent.AllowsService("chat") {
		t.Fatal("assistant permission grants direct chat access")
	}
}
