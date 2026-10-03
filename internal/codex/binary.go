package codex

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Resolve npm's entrypoint without running Node or mounting the package tree.
// Only the native executable is passed to the existing sandbox.
func resolveBinary(path, arch string) (string, error) {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if nativeBinary(path) {
		return path, nil
	}
	root := filepath.Dir(filepath.Dir(path))
	var pkg struct {
		Name string `json:"name"`
	}
	metadata, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || json.Unmarshal(metadata, &pkg) != nil || pkg.Name != "@openai/codex" || filepath.Base(path) != "codex.js" || filepath.Base(filepath.Dir(path)) != "bin" {
		return "", fmt.Errorf("%s is not a native Linux Codex executable or a recognised @openai/codex npm launcher; set CODEX_BINARY to the native executable", path)
	}
	target, platform := "", ""
	switch arch {
	case "amd64":
		target, platform = "x86_64-unknown-linux-musl", "codex-linux-x64"
	case "arm64":
		target, platform = "aarch64-unknown-linux-musl", "codex-linux-arm64"
	default:
		return "", fmt.Errorf("Codex preview does not support Linux architecture %s", arch)
	}
	// Node resolves optional platform packages in local or ancestor node_modules.
	var roots []string
	for dir := root; ; dir = filepath.Dir(dir) {
		roots = append(roots, filepath.Join(dir, "node_modules", "@openai", platform, "vendor"))
		if filepath.Dir(dir) == dir {
			break
		}
	}
	roots = append(roots, filepath.Join(root, "vendor"))
	for _, vendor := range roots {
		for _, subdir := range []string{"bin", "codex"} {
			candidate := filepath.Join(vendor, target, subdir, "codex")
			if nativeBinary(candidate) {
				return filepath.EvalSymlinks(candidate)
			}
		}
	}
	return "", fmt.Errorf("Codex npm launcher found at %s, but its native Linux %s binary is missing or not executable; reinstall @openai/codex@%s with optional dependencies, or set CODEX_BINARY to the standalone executable", path, arch, Version)
}

func nativeBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return false
	}
	var magic [4]byte
	_, err = io.ReadFull(f, magic[:])
	return err == nil && string(magic[:]) == "\x7fELF"
}
