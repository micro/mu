package test

// The deploy workflow skips a push that only touches files the running instance
// cannot serve. That list is a loaded gun: docs/*.md look like documentation and
// read like documentation, but they are compiled into the binary and served at
// /docs, so ignoring them would publish nothing while looking like it had.
//
// This test reads the workflow's paths-ignore list and every //go:embed
// directive in the tree, and fails if the two ever overlap.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// embeddedFiles returns every file compiled into the binary, found by reading
// the //go:embed directives rather than by keeping a second list here.
func embeddedFiles(t *testing.T) []string {
	t.Helper()

	var out []string
	err := filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "//go:embed ") {
				continue
			}
			for _, pattern := range strings.Fields(strings.TrimPrefix(line, "//go:embed ")) {
				// Patterns are relative to the file's own directory.
				matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), pattern))
				for _, m := range matches {
					if info, err := os.Stat(m); err == nil && info.IsDir() {
						filepath.Walk(m, func(p string, i os.FileInfo, e error) error {
							if e == nil && !i.IsDir() {
								out = append(out, filepath.ToSlash(p))
							}
							return nil
						})
						continue
					}
					out = append(out, filepath.ToSlash(m))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(out) < 10 {
		t.Fatalf("found only %d embedded files; the scan is not finding them", len(out))
	}
	return out
}

// ignoredPaths returns the deploy workflow's paths-ignore entries.
func ignoredPaths(t *testing.T) []string {
	t.Helper()

	b, err := os.ReadFile(at(".github/workflows/deploy.yml"))
	if err != nil {
		t.Fatalf("read deploy.yml: %v", err)
	}

	var out []string
	inList := false
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "paths-ignore:") {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				break // the list ended
			}
			continue
		}
		out = append(out, strings.Trim(strings.TrimPrefix(trimmed, "- "), `'"`))
	}
	return out
}

func TestDeployIgnoresOnlyUnservedFiles(t *testing.T) {
	ignored := ignoredPaths(t)
	if len(ignored) == 0 {
		t.Skip("the deploy workflow ignores nothing, so nothing can be wrongly ignored")
	}

	for _, file := range embeddedFiles(t) {
		for _, pattern := range ignored {
			if !ignoreMatches(pattern, file) {
				continue
			}
			t.Errorf("deploy.yml ignores %q, which covers %s — that file is embedded in "+
				"the binary, so a push changing it would never reach the live site",
				pattern, file)
		}
	}
}

// ignoreMatches reports whether a GitHub paths-ignore pattern covers a path.
// Only the two forms the workflow uses are handled: an exact path, and a
// `dir/**` prefix.
func ignoreMatches(pattern, path string) bool {
	if rest, ok := strings.CutSuffix(pattern, "/**"); ok {
		return path == rest || strings.HasPrefix(path, rest+"/")
	}
	return pattern == path
}

// The docs are the case worth naming outright: they are markdown, they live in a
// folder called docs, and they are served.
func TestDocsAreNeverIgnoredByDeploy(t *testing.T) {
	for _, pattern := range ignoredPaths(t) {
		if ignoreMatches(pattern, "docs/INSTALL.md") {
			t.Errorf("deploy.yml ignores %q — docs/*.md are embedded and served at /docs", pattern)
		}
	}
}
