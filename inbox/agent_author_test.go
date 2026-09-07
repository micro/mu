package inbox

import (
	"mu/internal/thread"
	"strings"
	"testing"
)

func TestMessagesShowTheirOwnAgent(t *testing.T) {
	old := AgentName
	AgentName = func(owner, id string) string {
		return map[string]string{"malten": "Malten", "research": "Research"}[id]
	}
	t.Cleanup(func() { AgentName = old })
	th := thread.Open(t.Name(), thread.ChatClient, "room")
	th.Agent = "research"
	got := messageBlock(t.Name(), th, thread.Message{Role: thread.RoleAgent, From: "malten", Text: "Taking that on"}, "")
	if !strings.Contains(got, "Malten") || strings.Contains(got, ">Agent<") || strings.Contains(got, "Research") {
		t.Fatalf("wrong agent: %s", got)
	}
}
