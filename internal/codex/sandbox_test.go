package codex

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSandboxMountsOnlyRuntimeAndFreshState(t *testing.T) {
	a := sandboxArgs("/private/codex", "/private/request", "/opt/codex", "app-server")
	joined := strings.Join(a, " ")
	for _, s := range []string{"--unshare-all", "--clearenv", "--die-with-parent", "--new-session", "--cap-drop ALL", "--tmpfs /tmp", "--proc /proc", "--setenv HOME /home/micro", "--setenv CODEX_HOME /state"} {
		if !strings.Contains(joined, s) {
			t.Fatalf("missing boundary %s", s)
		}
	}
	for i, s := range a {
		if s == "--bind" || s == "--ro-bind" {
			source := a[i+1]
			if source != "/usr" && source != "/lib" && source != "/lib64" && source != "/etc/ssl/certs" && source != "/etc/resolv.conf" && source != "/etc/hosts" && source != "/etc/nsswitch.conf" && source != "/private/codex" && source != "/private/request" {
				t.Fatalf("unexpected host mount %s", source)
			}
		}
	}
	if strings.Contains(joined, "--bind / ") || strings.Contains(joined, "--ro-bind / ") {
		t.Fatal("host root exposed")
	}
}

func TestFreshStateContainsNoPersonalContext(t *testing.T) {
	personal := t.TempDir()
	t.Setenv("CODEX_HOME", personal)
	os.WriteFile(filepath.Join(personal, "AGENTS.md"), []byte("PRIVATE"), 0600)
	os.MkdirAll(filepath.Join(personal, "memories"), 0700)
	a, e := temporaryState()
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(a)
	b, e := temporaryState()
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(b)
	if a == b {
		t.Fatal("reused state")
	}
	files, e := os.ReadDir(a)
	if e != nil || len(files) != 1 || files[0].Name() != "config.toml" {
		t.Fatal("personal state imported")
	}
	for _, setting := range []string{`"apps" = false`, `"plugins" = false`, `"memories" = false`, `"shell_tool" = false`, `"unified_exec" = false`, `"multi_agent" = false`, `"view_image" = false`, `"browser_use" = false`, `"computer_use" = false`, `use_memories = false`, `generate_memories = false`, `web_search = "disabled"`, `persistence = "none"`} {
		if !strings.Contains(string(config), setting) {
			t.Fatalf("missing isolation setting %s", setting)
		}
	}
}

func TestUnavailableSandboxNeverFallsBack(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux sandbox")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	_, close, e := start(context.Background())
	if e == nil || close != nil {
		t.Fatal("unrestricted fallback")
	}
	if Checked() {
		t.Fatal("failed sandbox became ready")
	}
}

func TestCredentialLockHonoursCancellation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux sandbox")
	}
	t.Setenv("HOME", t.TempDir())
	unlock, e := lock(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, e = lock(ctx); e == nil {
		t.Fatal("concurrent credential rotation")
	}
}
