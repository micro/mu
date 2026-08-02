package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// systemVars are read from the environment but are not Mu's settings — the OS
// provides them, and documenting them would be noise.
var systemVars = map[string]bool{
	"HOME": true, "PATH": true, "NO_COLOR": true, "XDG_CONFIG_HOME": true,
	"GNUPGHOME": true, "GPG_HOME": true, "GPG_KEYRING": true,
	"LISTEN_FDS": true, "LISTEN_PID": true, "ALREADY_SET": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	"http_proxy": true, "https_proxy": true, "no_proxy": true,
}

// Config is read directly and through a handful of small typed helpers. Each
// of those wraps os.Getenv, so scanning only for os.Getenv missed the settings
// they read — X402_NETWORK among them.
var configRead = regexp.MustCompile(`(?:settings\.Get|os\.Getenv|os\.LookupEnv|envOr|envInt|envIntAuth|getEnvInt)\("([A-Z][A-Z0-9_]*)"`)

// TestEveryConfigVarIsDocumented keeps the configuration page honest in both
// directions: a setting the code reads must be documented, and a setting the
// page lists must still be read. The previous page had drifted both ways —
// documenting variables that no longer existed while missing DISCORD_BOT_TOKEN
// and TELEGRAM_BOT_TOKEN entirely.
func TestEveryConfigVarIsDocumented(t *testing.T) {
	root := repoRoot(t)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "ENVIRONMENT_VARIABLES.md"))
	if err != nil {
		t.Fatalf("read config doc: %v", err)
	}
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("`([A-Z][A-Z0-9_]*)`").FindAllStringSubmatch(string(doc), -1) {
		documented[m[1]] = true
	}

	read := map[string]bool{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range configRead.FindAllStringSubmatch(string(b), -1) {
			if !systemVars[m[1]] {
				read[m[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(read) < 30 {
		t.Fatalf("only found %d config reads; the scan is broken", len(read))
	}

	// CREDIT_COST_<OP> is a family, one per priced operation. The page
	// documents the pattern and points at /pricing, which is generated from the
	// same catalogue the charges come from — twenty-five rows here would be a
	// second copy of a list that is already derived.
	family := func(v string) bool { return strings.HasPrefix(v, "CREDIT_COST_") }
	if !strings.Contains(string(doc), "CREDIT_COST_<OPERATION>") {
		t.Error("the CREDIT_COST_ override pattern is no longer documented")
	}

	for v := range read {
		if family(v) {
			continue
		}
		if !documented[v] {
			t.Errorf("%s is read by the code but not in ENVIRONMENT_VARIABLES.md", v)
		}
	}
	for v := range documented {
		if family(v) {
			continue
		}
		if !read[v] && !systemVars[v] {
			t.Errorf("%s is documented but nothing reads it", v)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found")
		}
		dir = parent
	}
}
