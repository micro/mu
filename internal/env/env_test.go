package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsUnsetKeysOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nexport X402_PAY_TO=0xabc\nX402_NETWORK=\"base\"\nALREADY_SET=fromfile\n\nBAD LINE\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ALREADY_SET", "fromenv")
	os.Unsetenv("X402_PAY_TO")
	os.Unsetenv("X402_NETWORK")

	Load(path)

	if got := os.Getenv("X402_PAY_TO"); got != "0xabc" {
		t.Errorf("X402_PAY_TO = %q, want 0xabc", got)
	}
	if got := os.Getenv("X402_NETWORK"); got != "base" {
		t.Errorf("X402_NETWORK = %q, want base (quotes stripped)", got)
	}
	if got := os.Getenv("ALREADY_SET"); got != "fromenv" {
		t.Errorf("ALREADY_SET = %q, want fromenv (env must win over file)", got)
	}
}

func TestLoadMissingFileIsNoop(t *testing.T) {
	Load(filepath.Join(t.TempDir(), "does-not-exist"))
}
