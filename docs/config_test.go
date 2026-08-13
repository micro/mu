package docs

import (
	"encoding/json"
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
var configRead = regexp.MustCompile(`(?:settings\.Get|os\.Getenv|os\.LookupEnv|envOr|envInt|EnvInt|envIntAuth|getEnvInt|envOverride|limitSetting)\("([A-Z][A-Z0-9_]*)"`)

// Prices are data now, so the variables that override them are named in
// quota.json rather than in any Go source. The loader reads
// os.Getenv(key) with key from the file, which no scan of the code can see —
// so the file is scanned too, and a price override is documented on the same
// terms as everything else.
var configInJSON = regexp.MustCompile(`"env"\s*:\s*"([A-Z][A-Z0-9_]*)"`)

// TestEveryConfigVarIsDocumented keeps the configuration page honest in both
// directions: a setting the code reads must be documented, and a setting the
// page lists must still be read. The previous page had drifted both ways —
// documenting variables that no longer existed while missing DISCORD_BOT_TOKEN
// and TELEGRAM_BOT_TOKEN entirely.
func TestEveryConfigVarIsDocumented(t *testing.T) {
	root := repoRoot(t)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "INSTALL.md"))
	if err != nil {
		t.Fatalf("read config doc: %v", err)
	}
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("`([A-Z][A-Z0-9_]*)`").FindAllStringSubmatch(string(doc), -1) {
		documented[m[1]] = true
	}

	read := map[string]bool{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// examples/ is its own module, and what it reads from the
			// environment belongs to the example program rather than to an
			// instance. The x402 agent takes a wallet key that way; listing it
			// on the operator's configuration page would offer a setting no
			// server has.
			if info.Name() == "examples" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		isGo := strings.HasSuffix(path, ".go")
		if !isGo && filepath.Base(path) != "quota.json" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		scan := configRead
		if !isGo {
			scan = configInJSON
		}
		for _, m := range scan.FindAllStringSubmatch(string(b), -1) {
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

	// CREDIT_COST_<OP> is a family, one per priced operation, named in
	// quota.json rather than listed here — twenty-six rows on
	// this page would be a second copy of a file already in the repo. What the
	// page owes the reader is where that file is and how it is overridden.
	family := func(v string) bool { return strings.HasPrefix(v, "CREDIT_COST_") }
	for _, want := range []string{"quota.json", "CREDIT_COST_"} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("the price configuration no longer mentions %q, so an operator "+
				"cannot find out where prices are set", want)
		}
	}

	// Names quota.json hands to the environment. quota reads them by the name
	// in the file rather than a literal in Go — env, free_env and limit_env —
	// so a scan for os.Getenv("X") cannot see them, and the honest answer is
	// that they are read, by the file that names them.
	for _, v := range quotaEnvNames(t, root) {
		read[v] = true
	}

	for v := range read {
		if family(v) {
			continue
		}
		if !documented[v] {
			t.Errorf("%s is read by the code but not in INSTALL.md", v)
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

// quotaEnvNames is every environment variable quota.json names as an override.
func quotaEnvNames(t *testing.T, root string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "quota.json"))
	if err != nil {
		t.Fatalf("cannot read quota.json: %v", err)
	}
	var f struct {
		Operations []struct {
			Env      string `json:"env"`
			FreeEnv  string `json:"free_env"`
			LimitEnv string `json:"limit_env"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("quota.json is not valid JSON: %v", err)
	}
	var out []string
	for _, o := range f.Operations {
		for _, v := range []string{o.Env, o.FreeEnv, o.LimitEnv} {
			if v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}
