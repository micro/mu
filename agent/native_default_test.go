package agent

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

// TestNativeEnabledDefault: the go-micro agent is the default; only an explicit
// falsey AGENT_NATIVE disables it.
func TestNativeEnabledDefault(t *testing.T) {
	defer settings.Set("AGENT_NATIVE", "")
	settings.Set("AGENT_NATIVE", "")
	if !nativeEnabled() {
		t.Fatal("native agent should be enabled by default")
	}
	for _, off := range []string{"off", "false", "0", "no", "OFF"} {
		settings.Set("AGENT_NATIVE", off)
		if nativeEnabled() {
			t.Fatalf("AGENT_NATIVE=%q should disable the native agent", off)
		}
	}
	settings.Set("AGENT_NATIVE", "on")
	if !nativeEnabled() {
		t.Fatal("AGENT_NATIVE=on should keep it enabled")
	}
}

func TestNativeAgentInstanceNameIsUnique(t *testing.T) {
	first := nativeAgentInstanceName()
	second := nativeAgentInstanceName()

	if first == "" || second == "" {
		t.Fatal("native agent instance names should not be empty")
	}
	if first == second {
		t.Fatalf("native agent instance names should be unique, got %q twice", first)
	}
	if !strings.HasPrefix(first, "assistant-") || !strings.HasPrefix(second, "assistant-") {
		t.Fatalf("native agent instance names should keep assistant prefix, got %q and %q", first, second)
	}
}
