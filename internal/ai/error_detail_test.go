package ai

import (
	"strings"
	"testing"
)

func TestProviderErrorKeepsQuotaDetailsAndRedactsCredentials(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "private-google-credential")
	input := `HTTP 429 RESOURCE_EXHAUSTED quota=GenerateRequestsPerDay limit=0 retryDelay=60s https://host/path?key=private-google-credential&model=gemini Authorization: Bearer other-secret x-api-key: third-secret`
	got := ProviderErrorDetail(input)
	for _, secret := range []string{"private-google-credential", "other-secret", "third-secret"} {
		if strings.Contains(got, secret) {
			t.Fatal("credential leaked")
		}
	}
	for _, want := range []string{"HTTP 429", "RESOURCE_EXHAUSTED", "GenerateRequestsPerDay", "limit=0", "retryDelay=60s", "model=gemini"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost diagnostic %s: %s", want, got)
		}
	}
}
