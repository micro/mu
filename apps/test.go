package apps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"

	"mu/internal/auth"
)

// TestResult contains the results of testing an app's API calls.
type TestResult struct {
	OK       bool            `json:"ok"`
	Issues   []string        `json:"issues"`
	APITests []APITestResult `json:"api_tests,omitempty"`
}

// APITestResult records one SDK call test with the actual response.
type APITestResult struct {
	Call     string `json:"call"`              // e.g. "mu.markets({category:'crypto'})"
	Path     string `json:"path"`              // e.g. "/markets?category=crypto"
	Status   int    `json:"status"`            // HTTP status code
	Response string `json:"response,omitempty"` // truncated JSON response
	Error    string `json:"error,omitempty"`
}

// TestApp tests an app by checking structure, extracting SDK calls,
// executing them server-side, and verifying field access patterns.
func TestApp(slug, authorID string) *TestResult {
	a := GetApp(slug)
	if a == nil {
		return &TestResult{Issues: []string{"App not found"}}
	}
	return TestHTML(a.HTML, authorID)
}

// TestHTML tests raw HTML without requiring a saved app.
func TestHTML(html, authorID string) *TestResult {
	result := &TestResult{OK: true}
	lower := strings.ToLower(html)

	// Structural checks
	if len(html) < 100 {
		result.Issues = append(result.Issues, "HTML too short — likely incomplete")
		result.OK = false
	}
	if !strings.Contains(lower, "<script") {
		result.Issues = append(result.Issues, "No <script> tag — app has no interactivity")
	}
	if strings.Contains(html, `src="/apps/sdk.js"`) || strings.Contains(html, `src='/apps/sdk.js'`) {
		result.Issues = append(result.Issues, "Remove <script src=\"/apps/sdk.js\"> — SDK is auto-injected")
	}

	// Extract and test SDK calls
	sdkCalls := extractSDKCalls(html)
	for _, sc := range sdkCalls {
		tr := executeSDKCall(sc, authorID)
		result.APITests = append(result.APITests, tr)
		if tr.Error != "" {
			result.OK = false
		}
	}

	// Check field access patterns against real responses
	for _, tr := range result.APITests {
		if tr.Response == "" || tr.Error != "" {
			continue
		}
		issues := checkFieldAccess(html, tr)
		result.Issues = append(result.Issues, issues...)
		if len(issues) > 0 {
			result.OK = false
		}
	}

	return result
}

type sdkCall struct {
	call string // full match, e.g. "mu.markets({category:'crypto'})"
	api  string // "markets", "news", "weather"
	path string // HTTP path to call
}

var sdkPatterns = []struct {
	re   *regexp.Regexp
	api  string
	path func([]string) string
}{
	{
		re:  regexp.MustCompile(`mu\.markets\s*\(\s*\{[^}]*category\s*:\s*['"](\w+)['"][^}]*\}\s*\)`),
		api: "markets",
		path: func(m []string) string {
			return "/markets?category=" + m[1]
		},
	},
	{
		re:   regexp.MustCompile(`mu\.markets\s*\(\s*\)`),
		api:  "markets",
		path: func(m []string) string { return "/markets" },
	},
	{
		re:   regexp.MustCompile(`mu\.news\s*\(\s*\)`),
		api:  "news",
		path: func(m []string) string { return "/news" },
	},
	{
		re:  regexp.MustCompile(`mu\.weather\s*\(\s*\{[^}]*\}\s*\)`),
		api: "weather",
		// Use London as test coords
		path: func(m []string) string { return "/weather?lat=51.5&lon=-0.12" },
	},
	{
		re:   regexp.MustCompile(`mu\.video\s*\(\s*\)`),
		api:  "video",
		path: func(m []string) string { return "/video" },
	},
	{
		re:   regexp.MustCompile(`mu\.social\s*\(\s*\)`),
		api:  "social",
		path: func(m []string) string { return "/social" },
	},
}

func extractSDKCalls(html string) []sdkCall {
	var calls []sdkCall
	seen := map[string]bool{}
	for _, p := range sdkPatterns {
		for _, match := range p.re.FindAllStringSubmatch(html, -1) {
			path := p.path(match)
			if seen[path] {
				continue
			}
			seen[path] = true
			calls = append(calls, sdkCall{
				call: match[0],
				api:  p.api,
				path: path,
			})
		}
	}
	return calls
}

func executeSDKCall(sc sdkCall, authorID string) APITestResult {
	tr := APITestResult{Call: sc.call, Path: sc.path}

	sess, err := auth.CreateSession(authorID)
	if err != nil {
		tr.Error = "auth failed"
		return tr
	}

	req, _ := http.NewRequest("GET", sc.path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	req.Header.Set("Accept", "application/json")

	recorder := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(recorder, req)

	tr.Status = recorder.Code
	if recorder.Code >= 400 {
		tr.Error = fmt.Sprintf("HTTP %d", recorder.Code)
		return tr
	}

	// Truncate response for inclusion in error context
	body := recorder.Body.String()
	if len(body) > 800 {
		body = body[:800] + "…"
	}
	tr.Response = body
	return tr
}

// checkFieldAccess verifies the JS code accesses response fields correctly.
func checkFieldAccess(html string, tr APITestResult) []string {
	var issues []string
	var response map[string]any
	if err := json.Unmarshal([]byte(tr.Response), &response); err != nil {
		return nil // can't parse, skip
	}

	switch {
	case strings.Contains(tr.Path, "/markets"):
		// Response is {category, data: [...]}
		// Common mistake: data.forEach() instead of data.data.forEach()
		if strings.Contains(html, "data.forEach") && !strings.Contains(html, "data.data.forEach") && !strings.Contains(html, "data.data.map") {
			if _, ok := response["data"]; ok {
				issues = append(issues, fmt.Sprintf(
					"Markets: code uses data.forEach() but response is {category, data: [...]}, use data.data.forEach(). Actual response: %s", truncateStr(tr.Response, 300)))
			}
		}
		// Also check: data.Symbol vs data.symbol (PascalCase vs snake_case)
		if strings.Contains(html, ".Symbol") || strings.Contains(html, ".Price") || strings.Contains(html, ".Change") {
			issues = append(issues, "Markets: fields are lowercase (symbol, price, change_24h), not PascalCase (Symbol, Price)")
		}

	case strings.Contains(tr.Path, "/news"):
		// Response is {feed: [...]}
		if strings.Contains(html, "data.forEach") && !strings.Contains(html, "data.feed.forEach") && !strings.Contains(html, "data.feed.map") {
			if _, ok := response["feed"]; ok {
				issues = append(issues, fmt.Sprintf(
					"News: code uses data.forEach() but response is {feed: [...]}, use data.feed.forEach(). Actual response: %s", truncateStr(tr.Response, 300)))
			}
		}

	case strings.Contains(tr.Path, "/weather"):
		// Response is {forecast: {Current: {...}, DailyItems: [...]}}
		if strings.Contains(html, "data.Current") && !strings.Contains(html, "data.forecast.Current") {
			issues = append(issues, fmt.Sprintf(
				"Weather: code uses data.Current but response is {forecast: {Current: {...}}}, use data.forecast.Current. Actual response: %s", truncateStr(tr.Response, 300)))
		}
	}

	return issues
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
