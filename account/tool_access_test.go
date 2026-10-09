package account

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolAccessNextSteps(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	for _, marker := range []string{"/account/tokens?access=services", "https://m3o.test/tools", "Manage billing here"} {
		if !strings.Contains(toolAccess(false), marker) {
			t.Errorf("missing account next step: %s", marker)
		}
	}
	r := httptest.NewRequest("GET", "https://micro.test/account/tokens?access=services", nil)
	body := apiTokenForm(r, "test")
	for _, marker := range []string{"https://m3o.test/mcp", "Bearer YOUR_TOKEN", "mcpServers", "account credits", "https://m3o.test/api"} {
		if !strings.Contains(body, marker) {
			t.Errorf("missing connection guidance: %s", marker)
		}
	}
	t.Setenv("X402_HOST", "")
	if toolAccess(false) != "" || toolAccess(true) != "" {
		t.Fatal("unconfigured host has onboarding")
	}
}
