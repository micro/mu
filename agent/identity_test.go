package agent

import (
	"mu/agent/micro"
	"strings"
	"testing"
)

func TestSelectedAgentIdentitySurvivesToolContext(t *testing.T) {
	for _, prompt := range []string{"", "Help build reflective anonymous streams."} {
		opts := PlatformOpts(&micro.Agent{ID: "malten", Name: "Malten", SystemPrompt: prompt})
		system := nativeSystem(opts)
		for _, want := range []string{"You are Malten", "not your identity", "do not claim their handles", prompt} {
			if !strings.Contains(system, want) {
				t.Fatalf("missing %q in %s", want, system)
			}
		}
		if strings.Contains(system, "You are Micro,") {
			t.Fatal("selected agent inherited default identity")
		}
	}
}
