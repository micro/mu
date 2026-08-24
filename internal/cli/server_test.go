package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every command calls the same instance.
//
// `mu agent` and `mu x402` each carried their own copy of the default URL and
// read neither the environment nor the config file, so somebody who had run
// `mu login https://their.host` had two commands still quietly calling
// micro.mu — with their wallet, on somebody else's instance. On an
// open-source, self-hostable binary that is the wrong default in the one place
// it matters.
func TestEveryCommandCallsTheSameInstance(t *testing.T) {
	var rc ResolvedConfig
	rc.Apply(&Config{URL: "https://their.host"})

	if got := rc.Server(""); got != "https://their.host" {
		t.Errorf("Server() = %q, want the configured instance", got)
	}
	// An explicit --server still wins, because it is the most local thing said.
	if got := rc.Server("https://other.host"); got != "https://other.host" {
		t.Errorf("Server(flag) = %q, want the flag", got)
	}
}

// And with nothing configured at all there is still an answer, because the
// first thing anybody does with a CLI is run it.
func TestWithNothingConfiguredTheDefaultAnswers(t *testing.T) {
	var rc ResolvedConfig
	if got := rc.Server(""); got != DefaultURL {
		t.Errorf("Server() = %q, want %q", got, DefaultURL)
	}
}

// The order is: --url, then MU_URL, then the file, then the default.
func TestTheOrderIsFlagEnvFileDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "mu"), 0o700); err != nil {
		t.Fatal(err)
	}
	file := &Config{URL: "https://from-file"}

	t.Setenv("MU_URL", "https://from-env")
	var env ResolvedConfig
	env.Apply(file)
	if env.Server("") != "https://from-env" {
		t.Errorf("env should beat the file, got %q", env.Server(""))
	}

	flag := ResolvedConfig{URL: "https://from-flag"}
	flag.Apply(file)
	if flag.Server("") != "https://from-flag" {
		t.Errorf("--url should beat the environment, got %q", flag.Server(""))
	}

	t.Setenv("MU_URL", "")
	var onlyFile ResolvedConfig
	onlyFile.Apply(file)
	if onlyFile.Server("") != "https://from-file" {
		t.Errorf("the file should beat the default, got %q", onlyFile.Server(""))
	}
}

// No command may hold its own copy of the address.
func TestNoCommandHardcodesTheInstance(t *testing.T) {
	for _, f := range []string{"agent.go", "x402pay.go", "ask.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue // the doc comments show example invocations
			}
			if strings.Contains(line, "micro.mu") {
				t.Errorf("%s:%d hardcodes the instance instead of asking "+
					"ResolvedConfig.Server:\n\t%s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
