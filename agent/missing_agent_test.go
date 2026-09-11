package agent

import (
	"strings"
	"testing"
)

func TestAskRefusesMissingAgentBeforeStartingConversation(t *testing.T) {
	id := owner(t, "missing_agent_ask")
	a, _, err := CreateAgent(id, "Temporary specialist", Hosted, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveAgent(id, a.ID); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{a.ID, "unknown-specialist"} {
		answer, err := Ask(AskRequest{Account: id, Agent: ref, Text: "List my mail"})
		if err == nil || !strings.Contains(err.Error(), "no agent") || answer.Thread != "" {
			t.Fatalf("missing agent started a conversation: %+v, %v", answer, err)
		}
	}
}
