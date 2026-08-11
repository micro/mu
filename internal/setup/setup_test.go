package setup

import (
	"testing"

	"mu/internal/settings"
)

func TestApplyProvider_OpenRouter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENROUTER_API_KEY", "")
	prev := settings.Get("OPENROUTER_API_KEY")
	t.Cleanup(func() { settings.Set("OPENROUTER_API_KEY", prev) })
	settings.Set("OPENROUTER_API_KEY", "")

	if err := ApplyProvider("openrouter", "", ""); err == nil {
		t.Fatal("empty key should fail")
	}
	if err := ApplyProvider("openrouter", "sk-or-test", ""); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}
	if got := settings.Get("OPENROUTER_API_KEY"); got != "sk-or-test" {
		t.Fatalf("OPENROUTER_API_KEY = %q", got)
	}
}

func TestApplyProvider_Unknown(t *testing.T) {
	if err := ApplyProvider("nope", "k", ""); err == nil {
		t.Fatal("unknown provider should fail")
	}
}
