package apps

import (
	"fmt"
	"regexp"
	"strings"
)

// These are diagnostics for known runtime mistakes, not a security boundary or
// a JavaScript interpreter. The sandbox continues to enforce access at runtime.
var scriptBody = regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script\s*>`)
var missingAPI = regexp.MustCompile(`\bmu\s*\.\s*api\s*\(`)
var directFetch = regexp.MustCompile("(?:^|[^.$\\w])(?:window\\.)?fetch\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]")
var fetchVariable = regexp.MustCompile(`(?:^|[^.$\w])(?:window\.)?fetch\s*\(\s*([A-Za-z_$][\w$]*)\s*[,)]`)
var urlVariable = regexp.MustCompile("\\b(?:const|let|var)\\s+([A-Za-z_$][\\w$]*)\\s*=\\s*['\"`]([^'\"`]+)['\"`]")

func runtimeIssues(document string) []string {
	var issues []string
	for _, script := range scriptBody.FindAllStringSubmatch(document, -1) {
		js := script[1]
		if missingAPI.MatchString(js) {
			issues = append(issues, "mu.api is not a function. Use mu.service(name, method, args), or fetch('/api/v1/<service>/<method>'); inspect the service response schema before reading fields.")
		}
		check := func(path string) {
			if !strings.HasPrefix(path, "/api/v1/") {
				issues = append(issues, fmt.Sprintf("fetch(%q) is blocked in the app sandbox. Use an available service through /api/v1/<service>/<method>. External requests require the supported mu.web.fetch proxy and its response contract; do not blame the user's connection.", path))
			}
		}
		for _, call := range directFetch.FindAllStringSubmatch(js, -1) {
			check(call[1])
		}
		variables := map[string]string{}
		for _, v := range urlVariable.FindAllStringSubmatch(js, -1) {
			variables[v[1]] = v[2]
		}
		for _, call := range fetchVariable.FindAllStringSubmatch(js, -1) {
			if path, ok := variables[call[1]]; ok {
				check(path)
			}
		}
	}
	return issues
}

const authoringContract = " Apps run in a sandbox. Use fetch('/api/v1/<service>/<method>') or mu.service(name, method, args); mu.api does not exist. Direct external fetch is blocked. Inspect real service schemas and data before coding; never invent fields, rankings or claim runtime verification from static checks."
