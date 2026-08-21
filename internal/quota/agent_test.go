package quota

import (
	"testing"

	"mu/internal/auth"
)

// TestAnAgentAccountIsRecordedNotCharged is the rule that replaced making the
// instance's agent an admin.
//
// The agent needs not to be *charged* — a billing property. It was got by
// granting admin, which is exempt for entirely different reasons and carries
// /admin/config, the console and the power to ban. Same outcome, wildly different
// authority.
func TestAnAgentAccountIsRecordedNotCharged(t *testing.T) {
	if err := LoadFromTree(); err != nil {
		t.Fatalf("could not load quota.json: %v", err)
	}
	prev := Enabled
	Enabled = func() bool { return true }
	t.Cleanup(func() { Enabled = prev })

	const id = "some-agent"
	auth.SetAccountForTest(&auth.Account{ID: id, Name: "Some agent", Agent: true})
	t.Cleanup(func() { auth.RemoveAccountForTest(id) })

	// web_search costs real money and this account has no balance at all.
	if cost := OperationCost(OpWebSearch); cost == 0 {
		t.Fatal("web_search is priced at zero, so this proves nothing")
	}
	ok, _, cost, err := CheckQuota(id, OpWebSearch)
	if !ok {
		t.Fatalf("an agent account was refused a metered call: cost %d, %v", cost, err)
	}
	if cost != 0 {
		t.Errorf("an agent account was quoted %d credits", cost)
	}

	// A person with the same empty balance is refused, which is the half that
	// must not have been loosened. There used to be a daily grant of credits to
	// spend down first before this held; it is gone, so an empty balance is an
	// empty balance. The agent's exemption is that it is never charged at all,
	// which is a different thing and the thing under test.
	const human = "some-person"
	auth.SetAccountForTest(&auth.Account{ID: human, Name: "Some person"})
	t.Cleanup(func() { auth.RemoveAccountForTest(human) })
	ResetAllowances()
	t.Cleanup(ResetAllowances)
	if ok, _, _, _ := CheckQuota(human, OpWebSearch); ok {
		t.Error("a person with no credits was allowed a metered call")
	}
	// And the agent still is not.
	if ok, _, _, _ := CheckQuota(id, OpWebSearch); !ok {
		t.Error("an agent account was refused once a person's allowance ran out")
	}
}
