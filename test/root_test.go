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

import "path/filepath"

// repo is the repository root, from this package's directory.
const repo = ".."

// at joins a repo-relative path. Use it for every file these tests open.
func at(parts ...string) string {
	return filepath.Join(append([]string{repo}, parts...)...)
}
