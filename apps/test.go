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
	Total   int          `json:"total"`
	Passed  int          `json:"passed"`
	Failed  int          `json:"failed"`
	Results []APICallResult `json:"results"`
	Issues  []string     `json:"issues"`
}

// APICallResult records one API call test.
type APICallResult struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status string `json:"status"` // "ok" or "error"
	Error  string `json:"error,omitempty"`
}

// TestApp extracts mu.api calls from an app's HTML/JS code,
// executes them server-side, and reports which ones work.
func TestApp(slug, authorID string) *TestResult {
	a := GetApp(slug)
	if a == nil {
		return &TestResult{Issues: []string{"App not found"}}
	}

	result := &TestResult{}
	html := a.HTML

	// Structural checks
	lower := strings.ToLower(html)
	if len(html) < 100 {
		result.Issues = append(result.Issues, "HTML is too short — likely incomplete")
	}
	if !strings.Contains(lower, "<body") {
		result.Issues = append(result.Issues, "Missing <body> tag")
	}
	if !strings.Contains(lower, "<script") {
		result.Issues = append(result.Issues, "No JavaScript — app has no interactivity")
	}

	// Check for common mistakes
	if strings.Contains(html, "fetch('http") || strings.Contains(html, `fetch("http`) {
		result.Issues = append(result.Issues, "Uses fetch() to call external URLs — blocked by sandbox. Use mu.api instead.")
	}
	if strings.Contains(lower, `<script src="http`) || strings.Contains(lower, `<script src='http`) {
		result.Issues = append(result.Issues, "Loads external scripts — blocked by sandbox CSP")
	}
	if strings.Contains(html, "/weather?q=") || strings.Contains(html, "/weather?city=") {
		result.Issues = append(result.Issues, "Weather API needs lat/lon not city name. Use /weather?lat=NUMBER&lon=NUMBER")
	}
	if strings.Contains(html, `src="/apps/sdk.js"`) || strings.Contains(html, `src='/apps/sdk.js'`) {
		result.Issues = append(result.Issues, "SDK script tag included — it's auto-injected, remove the <script src> tag")
	}

	// Extract mu.api.get() calls and test them
	getPattern := regexp.MustCompile(`mu\.api\.get\(['"]([^'"]+)['"]\)`)
	for _, match := range getPattern.FindAllStringSubmatch(html, -1) {
		if len(match) < 2 {
			continue
		}
		path := match[1]
		result.Total++

		// Skip dynamic paths
		if strings.Contains(path, "${") || strings.Contains(path, "'+") || strings.Contains(path, `"+`) {
			result.Results = append(result.Results, APICallResult{
				Method: "GET", Path: path, Status: "skip",
				Error: "Dynamic path — can't test statically",
			})
			continue
		}

		// Test by making an internal HTTP request
		callResult := testAPICall(authorID, "GET", path)
		result.Results = append(result.Results, callResult)
		if callResult.Status == "ok" {
			result.Passed++
		} else {
			result.Failed++
			result.Issues = append(result.Issues, fmt.Sprintf("GET %s: %s", path, callResult.Error))
		}
	}

	// Extract mu.api.post() calls
	postPattern := regexp.MustCompile(`mu\.api\.post\(['"]([^'"]+)['"]`)
	for _, match := range postPattern.FindAllStringSubmatch(html, -1) {
		if len(match) < 2 {
			continue
		}
		path := match[1]
		result.Total++
		// Just check the endpoint is valid — don't execute POST with unknown body
		validPaths := map[string]bool{
			"/places/search": true, "/places/nearby": true,
			"/chat": true, "/news": true,
		}
		if validPaths[path] {
			result.Passed++
			result.Results = append(result.Results, APICallResult{
				Method: "POST", Path: path, Status: "ok",
			})
		} else {
			result.Results = append(result.Results, APICallResult{
				Method: "POST", Path: path, Status: "skip",
				Error: "Unknown POST endpoint",
			})
		}
	}

	return result
}

// testAPICall makes an internal HTTP request to a Mu API endpoint
// and checks if it returns a successful response.
func testAPICall(authorID, method, path string) APICallResult {
	// Create authenticated request
	sess, err := auth.CreateSession(authorID)
	if err != nil {
		return APICallResult{Method: method, Path: path, Status: "error", Error: "auth failed"}
	}

	req, _ := http.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	req.Header.Set("Accept", "application/json")

	recorder := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(recorder, req)

	if recorder.Code >= 400 {
		// Try to extract error message from JSON response
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(recorder.Body.Bytes(), &errResp) == nil && errResp.Error != "" {
			return APICallResult{Method: method, Path: path, Status: "error", Error: fmt.Sprintf("HTTP %d: %s", recorder.Code, errResp.Error)}
		}
		return APICallResult{Method: method, Path: path, Status: "error", Error: fmt.Sprintf("HTTP %d", recorder.Code)}
	}

	return APICallResult{Method: method, Path: path, Status: "ok"}
}
