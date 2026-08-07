package ai

import (
	"errors"
	"testing"
)

// The status page asks whether the model answers, not whether a key is set.
//
// It used to ask Configured(), which is a statement about this process's
// environment: an instance pointed at an Ollama that is not running, or holding
// a key that has expired, reported "All systems operational" while every
// question a user asked came back "Could not generate response".
func TestHealthyReportsTheLastRealCall(t *testing.T) {
	defer resetHealth()

	// Nothing tried yet: fall back to configuration, because "not asked" is not
	// the same as "broken".
	health.tried = false
	if ok, _ := Healthy(); ok != Configured() {
		t.Fatal("with no call yet, health should follow configuration")
	}

	recordHealth(errors.New("connection refused"))
	ok, why := Healthy()
	if ok {
		t.Fatal("a failed call should make the agent unhealthy")
	}
	if why != "connection refused" {
		t.Fatalf("expected the reason to be carried through, got %q", why)
	}

	// And it recovers on the next success rather than latching.
	recordHealth(nil)
	if ok, why := Healthy(); !ok || why != "" {
		t.Fatalf("a successful call should clear the failure, got ok=%v why=%q", ok, why)
	}
}

// Status names the provider a question would actually go to. It used to look
// only for ANTHROPIC_API_KEY, so a healthy Atlas Cloud or local instance
// reported "Not configured" and dragged the whole instance's health down.
func TestStatusFollowsHealth(t *testing.T) {
	defer resetHealth()

	recordHealth(errors.New("boom"))
	desc, ok := Status()
	if ok {
		t.Fatal("Status should report unhealthy after a failed call")
	}
	if desc == "" {
		t.Fatal("Status should always describe something")
	}
}

// resetHealth puts the recorder back to "nothing tried yet" so tests do not
// leak a verdict into each other.
func resetHealth() {
	health.Lock()
	defer health.Unlock()
	health.tried, health.ok, health.err = false, false, ""
}
