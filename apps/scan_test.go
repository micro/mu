package apps

import (
	"strings"
	"testing"
)

func TestScanAppAllowsBenignHTML(t *testing.T) {
	issues := ScanApp(`<!doctype html><button onclick="window.location='/apps'">Apps</button><script>console.log('ok')</script>`)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %v", issues)
	}
}

func TestScanAppBlocksCookieAccessCaseInsensitive(t *testing.T) {
	issues := ScanApp(`<script>console.log(Document.Cookie)</script>`)
	assertScanIssue(t, issues, "document.cookie")
}

func TestScanAppBlocksCredentialHarvestingCaseInsensitive(t *testing.T) {
	issues := ScanApp(`<script>var xhr = new xmlhttprequest(); xhr.send(password)</script>`)
	assertScanIssue(t, issues, "credential")
}

func TestScanAppBlocksExternalRedirectsWithSpacing(t *testing.T) {
	cases := []string{
		`<script>window.location = "https://evil.example"</script>`,
		`<script>window.location.href = 'http://evil.example'</script>`,
		`<script>WINDOW.LOCATION.HREF='https://evil.example'</script>`,
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			issues := ScanApp(tc)
			assertScanIssue(t, issues, "Redirecting to external URLs")
		})
	}
}

func TestScanAppBlocksExternalScriptSources(t *testing.T) {
	cases := []string{
		`<script src = "https://cdn.example/app.js"></script>`,
		`<SCRIPT async src='http://cdn.example/app.js'></SCRIPT>`,
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			issues := ScanApp(tc)
			assertScanIssue(t, issues, "Loading external scripts")
		})
	}
}

func TestScanAppBlocksEvalCaseInsensitiveUnlessAllowed(t *testing.T) {
	issues := ScanApp(`<script>EVAL(userInput)</script>`)
	assertScanIssue(t, issues, "eval()")

	issues = ScanApp(`<script>// SAFE-EVAL
	eval(trustedCode)</script>`)
	if len(issues) != 0 {
		t.Fatalf("expected safe-eval marker to allow eval, got %v", issues)
	}
}

func assertScanIssue(t *testing.T, issues []string, want string) {
	t.Helper()
	for _, issue := range issues {
		if strings.Contains(issue, want) {
			return
		}
	}
	t.Fatalf("expected issue containing %q, got %v", want, issues)
}
