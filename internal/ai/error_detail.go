package ai

import (
	"mu/internal/settings"
	"regexp"
	"strings"
)

var credentialField = regexp.MustCompile(`(?i)((?:[?&]|\b)(?:key|api[_-]?key|x-goog-api-key|x-api-key|access_token|token|authorization)["']?\s*[:=]\s*["']?)(?:bearer\s+)?[^\s&"'<>},]+`)
var bearerCredential = regexp.MustCompile(`(?i)(\bbearer\s+)[a-z0-9._~+/=-]+`)

// ProviderErrorDetail retains diagnostics while removing configured credentials
// and credentials embedded in URLs or headers by a provider's HTTP client.
func ProviderErrorDetail(detail string) string {
	for _, key := range []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY", "ATLASCLOUD_API_KEY", "ATLAS_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY"} {
		if secret := settings.Get(key); secret != "" {
			detail = strings.ReplaceAll(detail, secret, "[redacted]")
		}
	}
	detail = credentialField.ReplaceAllString(detail, "${1}[redacted]")
	return bearerCredential.ReplaceAllString(detail, "${1}[redacted]")
}
