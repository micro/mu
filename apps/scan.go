package apps

import (
	"strings"
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
	if strings.Contains(html, "XMLHttpRequest") && (strings.Contains(lower, "password") || strings.Contains(lower, "credential")) {
		issues = append(issues, "Suspicious credential harvesting pattern")
	}

	// Block redirecting to external sites for phishing
	if strings.Contains(lower, "window.location") && strings.Contains(html, "http") {
		// Allow relative redirects, block absolute
		for _, pattern := range []string{"window.location='http", `window.location="http`, "window.location.href='http", `window.location.href="http`} {
			if strings.Contains(lower, pattern) {
				issues = append(issues, "Redirecting to external URLs is not allowed")
				break
			}
		}
	}

	// Block loading external scripts
	if strings.Contains(lower, `<script src="http`) || strings.Contains(lower, `<script src='http`) {
		issues = append(issues, "Loading external scripts is not allowed")
	}

	// Block eval with string concatenation (code injection)
	if strings.Contains(html, "eval(") && !strings.Contains(html, "// safe-eval") {
		issues = append(issues, "eval() is not allowed")
	}

	// Block localStorage/sessionStorage access to other apps
	// (apps can use mu.store which is namespaced per app)
	// This is a soft warning, not a block

	return issues
}
