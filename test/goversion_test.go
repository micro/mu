package test

// One Go version, in the five places that claim one.
//
// go.mod is the only one that is enforced by anything: the toolchain reads it
// and either downloads what it asks for or refuses. Everything else is a copy —
// the Dockerfile's base image, two setup-go steps, and the requirement line a
// self-hoster reads before they start — and copies drift silently.
//
// They had. go.mod was raised to 1.26 for chromedp and the other four were left
// at 1.25, so CI and the release workflow were building a 1.26 module on a 1.25
// runner (quietly downloading a toolchain on every run), the Dockerfile pinned a
// base image that could not build it without doing the same, and the install
// guide told people they needed a version that is not enough.
//
// Nothing here checks that the number is right. It checks that there is one
// number.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEveryPlaceThatNamesAGoVersionAgrees(t *testing.T) {
	root := at("")

	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindStringSubmatch(string(mod))
	if m == nil {
		t.Fatal("go.mod does not declare a go version")
	}
	want := m[1]

	// Each file, and the pattern that carries the claim in it.
	for _, c := range []struct{ file, pattern string }{
		{"Dockerfile", `FROM golang:(\d+\.\d+)`},
		{".github/workflows/test.yml", `go-version: '(\d+\.\d+)'`},
		{".github/workflows/release.yml", `go-version: '(\d+\.\d+)'`},
		{"docs/INSTALL.md", `\*\*Go (\d+\.\d+)\+\*\*`},
	} {
		b, err := os.ReadFile(filepath.Join(root, c.file))
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		found := regexp.MustCompile(c.pattern).FindAllStringSubmatch(string(b), -1)
		if len(found) == 0 {
			t.Errorf("%s no longer names a Go version — either it stopped needing one, "+
				"or the pattern here has gone stale and is now checking nothing", c.file)
			continue
		}
		for _, f := range found {
			if f[1] != want {
				t.Errorf("%s says Go %s, go.mod says %s — CI, the released binaries and "+
					"the install guide have to agree with the module or a self-hoster "+
					"hits a toolchain they were not told to have",
					c.file, f[1], want)
			}
		}
	}

	// And the requirement line has to be the version, not a floor somebody
	// raised go.mod past.
	install, err := os.ReadFile(filepath.Join(root, "docs", "INSTALL.md"))
	if err == nil && !strings.Contains(string(install), "Go "+want+"+") {
		t.Errorf("the install guide's requirement is not Go %s+", want)
	}
}
