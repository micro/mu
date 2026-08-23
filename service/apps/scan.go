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
//
// Advisory, and until recently unused: this had a test suite and no caller
// anywhere in the tree, which is worse than having no scanner, because a
// scanner nothing runs reads as protection. It is the build loop's first
// question now — see buildProblems, where what it finds is handed back to the
// model as something to fix.
//
// Still not a gate on create or edit. What stops an app doing damage is the
// sandbox: an opaque origin with connect-src 'none', so it cannot read a
// cookie or reach anything. These patterns are the ones worth refusing to
// *write* rather than merely to contain, and turning them into a refusal on
// every save would reject working apps for a resemblance.
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
