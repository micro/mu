package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Implementation vocabulary must not reach the product surface. A user should
// never meet "go-micro", "microservice" or "framework" in the UI, in copy, or
// in anything the agent says about itself.
//
// "service" is deliberately NOT banned — it is ordinary English, and the
// internal naming convention (service name == route == nav label == tool
// prefix) is good and invisible to users.
//
// Developer-facing files are exempt: the README and docs are the funnel, and
// they are facts. The distinction is who reads it, not where it lives.
func TestNoImplementationVocabularyInProductSurface(t *testing.T) {
	banned := regexp.MustCompile(`(?i)go-micro|go micro|microservice|gomicro`)

	// Files whose strings reach a user: page copy, agent system prompts,
	// generated content, and the public status page.
	surface := []string{
		"../../home/landing.go",
		"../../home/pricing.go",
		"../../home/home.go",
		"../../internal/app/landing.go",
		"../../internal/app/status.go",
		"../../agent/agent.go",
		"../../agent/run.go",
		"../../agent/native.go",
		"../../blog/notes.go",
	}

	for _, path := range surface {
		b, err := os.ReadFile(path)
		if err != nil {
			continue // file moved or renamed; other tests will catch that
		}
		for i, line := range strings.Split(string(b), "\n") {
			if !banned.MatchString(line) {
				continue
			}
			trimmed := strings.TrimSpace(line)
			// A Go comment explaining the architecture is not product surface.
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			// "go-micro.dev/..." is a module path — an import or a qualified
			// type, i.e. code. Only prose reaches a user.
			if strings.Contains(line, "go-micro.dev/") {
				continue
			}
			t.Errorf("%s:%d exposes implementation vocabulary to users:\n\t%s",
				filepath.Base(path), i+1, strings.TrimSpace(line))
		}
	}
}

// The health check names on /status are rendered on a page linked from every
// footer, so they are copy, not internals.
func TestStatusCheckNamesAreUserFacing(t *testing.T) {
	b, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Skip("main.go not readable")
	}
	checkLine := regexp.MustCompile(`check\{"([^"]+)"`)
	banned := regexp.MustCompile(`(?i)go-micro|microservice|framework`)

	for i, line := range strings.Split(string(b), "\n") {
		m := checkLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if banned.MatchString(m[1]) {
			t.Errorf("main.go:%d status check named %q leaks implementation vocabulary onto /status",
				i+1, m[1])
		}
	}
}
