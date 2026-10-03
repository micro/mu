package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeAndNpmBinaryResolution(t *testing.T) {
	for _, layout := range []string{"standalone", "bundled", "nested", "hoisted"} {
		t.Run(layout, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "node_modules", "@openai", "codex")
			write := func(p, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0755); err != nil {
					t.Fatal(err)
				}
			}
			launcher := filepath.Join(root, "bin", "codex.js")
			write(launcher, "#!/usr/bin/env node\nthrow new Error('must never execute');")
			write(filepath.Join(root, "package.json"), `{"name":"@openai/codex"}`)
			vendor := filepath.Join(root, "vendor")
			if layout == "nested" {
				vendor = filepath.Join(root, "node_modules", "@openai", "codex-linux-x64", "vendor")
			}
			if layout == "hoisted" {
				vendor = filepath.Join(base, "node_modules", "@openai", "codex-linux-x64", "vendor")
			}
			binary := filepath.Join(vendor, "x86_64-unknown-linux-musl", "bin", "codex")
			if layout == "bundled" {
				binary = filepath.Join(vendor, "x86_64-unknown-linux-musl", "codex", "codex")
			}
			write(binary, "\x7fELFfixture")
			entry := filepath.Join(base, "codex")
			target := launcher
			if layout == "standalone" {
				target = binary
			}
			if err := os.Symlink(target, entry); err != nil {
				t.Fatal(err)
			}
			got, err := resolveBinary(entry, "amd64")
			if err != nil || got != binary {
				t.Fatalf("got %q, %v", got, err)
			}
			if layout != "standalone" {
				if _, err := resolveBinary(entry, "arm64"); err == nil {
					t.Fatal("selected wrong architecture")
				}
				os.Chmod(binary, 0644)
				if _, err := resolveBinary(entry, "amd64"); err == nil {
					t.Fatal("accepted non-executable")
				}
			}
		})
	}
}

func TestRejectUnknownLauncher(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755)
	if _, err := resolveBinary(path, "amd64"); err == nil {
		t.Fatal("unknown launcher accepted")
	}
}
