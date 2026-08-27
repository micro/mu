package test

// A link to an app goes to the app.
//
// "run" was this service's word for two things — showing an app, and parking a
// snippet of JavaScript at a temporary URL — and both are retired. Nothing
// routes /apps/<slug>/run: it falls through to the default branch, which takes
// the whole tail as the slug, fails to find "<slug>/run", and answers 404.
//
// The word was retired in the handler and left behind in three places that
// build links: the Launch button on an app's own page, the app list on every
// profile, and the description the agent hands out when somebody asks about an
// app. Each was a separate 404, and the last one is an agent telling a person
// to click something broken.
//
// One implementation, three doors. This is the thing that closes them.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNothingLinksToTheRetiredRunPath(t *testing.T) {
	root := ".."
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(body), "\n") {
			// Comments explain why the path is gone; they are not links to it.
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if strings.Contains(line, `/apps/`) && strings.Contains(line, `/run`) {
				offenders = append(offenders,
					fmt.Sprintf("%s:%d: %s", filepath.ToSlash(path), i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	for _, o := range offenders {
		t.Errorf("links to /apps/<slug>/run, which 404s — the app is at /apps/<slug>:\n  %s", o)
	}
}
