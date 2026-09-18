package chat

import (
	"mu/internal/auth"
	"testing"
)

func TestMicroDMRequiresAgentAndExactPair(t *testing.T) {
	if err := auth.Create(&auth.Account{ID: auth.MicroID, Agent: true}); err != nil {
		t.Fatal(err)
	}
	memberMu.Lock()
	members["dm_agent_test"] = []string{"alice", auth.MicroID}
	members["dm_people_test"] = []string{"alice", "bob"}
	members["dm_group_test"] = []string{"alice", "bob", auth.MicroID}
	memberMu.Unlock()
	for _, tc := range []struct {
		room, sender string
		want         bool
	}{
		{"dm_agent_test", "alice", true},
		{"dm_agent_test", "stranger", false},
		{"dm_agent_test", auth.MicroID, false},
		{"dm_people_test", "alice", false},
		{"dm_group_test", "alice", false},
		{"chat_topic", "alice", false},
		{"dm_missing", "alice", false},
	} {
		if got := microDM(tc.room, tc.sender); got != tc.want {
			t.Errorf("%s/%s: got %v", tc.room, tc.sender, got)
		}
	}
	acc, _ := auth.GetAccount(auth.MicroID)
	acc.Agent = false
	auth.UpdateAccount(acc)
	if microDM("dm_agent_test", "alice") {
		t.Fatal("human account named micro must not trigger agent")
	}
}
