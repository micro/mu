package test

// The tests that check the repository against itself.
//
// These read the source, the docs and the workflow file and compare them with
// the registry: which services exist, what the tools are called, whether the
// README still names tools that were removed, whether the deploy workflow
// ignores a path something embeds. They are checks on the repo rather than on
// a package, which is why they live in one place instead of being scattered
// through the packages they happen to import.
//
// They used to sit at the repository root as `package main`, six files deep,
// next to main.go. Nothing about them needs to be there: not one touches an
// unexported identifier in main. main_test.go does — argFloat, isServerMode,
// chargedWriteOp — and Go requires a test to live beside the package it tests,
// so that one file stays where it is and this is the rest.
//
// Everything here reads paths relative to the repo root rather than the
// working directory, because the working directory is now this folder.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo is the repository root, from this package's directory.
const repo = ".."

// at joins a repo-relative path. Use it for every file these tests open.
func at(parts ...string) string {
	return filepath.Join(append([]string{repo}, parts...)...)
}

// registrationSource is every file that can register a tool by hand, as one
// string.
//
// Tools declared on a Spec are in this binary's registry and can be asked
// directly; the hand-written ones are registered at startup, so the source is
// the only place a test can see them. That source used to be main.go, and
// three tests read it by name — so moving it to internal/server broke all
// three at once, for a change that altered nothing about the tools.
//
// Reading the directory rather than a filename means the next move is free.
func registrationSource(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, dir := range []string{"internal/server", "internal/api"} {
		entries, err := os.ReadDir(at(dir))
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
				strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			src, err := os.ReadFile(at(dir, e.Name()))
			if err != nil {
				t.Fatalf("reading %s/%s: %v", dir, e.Name(), err)
			}
			b.Write(src)
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		t.Fatal("no registration source found — the tool tests would pass vacuously")
	}
	return b.String()
}
