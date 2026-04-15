package cli

import (
	"os"
	"testing"
)

func TestResolvedConfigApply_Defaults(t *testing.T) {
	t.Setenv("MU_URL", "")
	t.Setenv("MU_TOKEN", "")

	var rc ResolvedConfig
	rc.Apply(nil)

	if rc.URL != DefaultURL {
		t.Errorf("URL = %q, want %q", rc.URL, DefaultURL)
	}
	if rc.Token != "" {
		t.Errorf("Token = %q, want empty", rc.Token)
	}
}

func TestResolvedConfigApply_File(t *testing.T) {
	t.Setenv("MU_URL", "")
	t.Setenv("MU_TOKEN", "")

	file := &Config{URL: "https://my.mu", Token: "abc"}
	var rc ResolvedConfig
	rc.Apply(file)

	if rc.URL != "https://my.mu" {
		t.Errorf("URL = %q, want https://my.mu", rc.URL)
	}
	if rc.Token != "abc" {
		t.Errorf("Token = %q, want abc", rc.Token)
	}
}

func TestResolvedConfigApply_EnvOverridesFile(t *testing.T) {
	t.Setenv("MU_URL", "https://env.mu")
	t.Setenv("MU_TOKEN", "env-tok")

	file := &Config{URL: "https://file.mu", Token: "file-tok"}
	var rc ResolvedConfig
	rc.Apply(file)

	if rc.URL != "https://env.mu" {
		t.Errorf("URL = %q, want https://env.mu", rc.URL)
	}
	if rc.Token != "env-tok" {
		t.Errorf("Token = %q, want env-tok", rc.Token)
	}
}

func TestResolvedConfigApply_FlagOverridesEnv(t *testing.T) {
	t.Setenv("MU_URL", "https://env.mu")
	t.Setenv("MU_TOKEN", "env-tok")

	rc := ResolvedConfig{URL: "https://flag.mu", Token: "flag-tok"}
	rc.Apply(&Config{URL: "https://file.mu", Token: "file-tok"})

	if rc.URL != "https://flag.mu" {
		t.Errorf("URL = %q, want https://flag.mu", rc.URL)
	}
	if rc.Token != "flag-tok" {
		t.Errorf("Token = %q, want flag-tok", rc.Token)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	c, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("nil config")
	}
	if c.URL != "" {
		t.Errorf("URL = %q, want empty", c.URL)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	want := &Config{URL: "https://mu.xyz", Token: "tok123"}
	if err := SaveConfig(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.URL != want.URL || got.Token != want.Token {
		t.Errorf("round-trip mismatch:\n want %+v\n  got %+v", want, got)
	}

	// Config file should have restrictive permissions.
	path, _ := configPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("permissions = %v, want 0600", mode)
	}
}
