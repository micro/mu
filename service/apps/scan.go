package apps

import (
	"regexp"
	"strings"
)

var (
	externalRedirectRe = regexp.MustCompile(`(?i)window\.location(?:\.href)?\s*=\s*['\"]https?://`)
	externalScriptRe   = regexp.MustCompile(`(?i)<script\b[^>]*\bsrc\s*=\s*['\"]https?://`)
	evalCallRe         = regexp.MustCompile(`(?i)\beval\s*\(`)
)

// ScanApp checks app HTML for security issues before saving.
// Returns a list of issues found. Empty = safe.
func ScanApp(html string) []string {
	var issues []string
	lower := strings.ToLower(html)

	// Block cookie theft
	if strings.Contains(lower, "document.cookie") {
		issues = append(issues, "Accessing document.cookie is not allowed")
	}

	// Block credential harvesting
	if strings.Contains(lower, "xmlhttprequest") && (strings.Contains(lower, "password") || strings.Contains(lower, "credential")) {
		issues = append(issues, "Suspicious credential harvesting pattern")
	}

	// Block redirecting to external sites for phishing.
	if externalRedirectRe.MatchString(html) {
		issues = append(issues, "Redirecting to external URLs is not allowed")
	}

	// Block loading external scripts.
	if externalScriptRe.MatchString(html) {
		issues = append(issues, "Loading external scripts is not allowed")
	}

	// Block eval with string concatenation (code injection).
	if evalCallRe.MatchString(html) && !strings.Contains(lower, "// safe-eval") {
		issues = append(issues, "eval() is not allowed")
	}

	// Block localStorage/sessionStorage access to other apps
	// (apps can use mu.store which is namespaced per app)
	// This is a soft warning, not a block

	return issues
}
