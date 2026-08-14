package events

// Running a standing instruction.
//
// An event with a prompt is not a reminder — it is the agent doing a piece of
// work while nobody is watching, which is the most valuable thing this instance
// does and was the only model call it did not charge for. A task run charges an
// agent query; a scheduled run is the same model doing the same work, and the
// rule is that credits price a real cost.
//
// The agent is reached through a hook main() sets, so this package does not
// import the agent and the agent knows nothing about events.

import (
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
)

// RunAgent is set by main() to run a prompt through the agent as an account.
var RunAgent func(accountID, prompt string) (string, error)

// RunPrompt runs a fired event's standing instruction and returns the text to
// deliver to its owner.
//
// It always returns something to send. A run that could not happen — no
// credits, no agent, a model error — is news the owner needs: a standing
// instruction that goes quiet looks like the instruction was forgotten, and
// they would have no way to tell the difference.
func RunPrompt(e *Event) string {
	prompt := strings.TrimSpace(e.Prompt)
	if prompt == "" {
		return ""
	}
	if RunAgent == nil {
		return "This scheduled task did not run: no agent is configured on this instance."
	}

	// Charged before the model is asked, so a run that cannot be paid for does
	// not spend one first.
	canProceed, _, cost, err := quota.CheckQuota(e.Owner, quota.OpAgentQuery)
	if err != nil {
		app.Log("events", "standing instruction %q could not be checked for %s: %v", e.Title, e.Owner, err)
		return "This scheduled task did not run: " + err.Error()
	}
	if !canProceed {
		return fmt.Sprintf("This scheduled task did not run: it costs %d credits and your balance is short. "+
			"Top up at /account/topup and it will run at its next time.", cost)
	}

	answer, err := RunAgent(e.Owner, prompt)
	if err != nil {
		// Not charged: nothing was produced.
		app.Log("events", "standing instruction %q failed for %s: %v", e.Title, e.Owner, err)
		return "This scheduled task failed: " + err.Error()
	}

	if err := quota.ConsumeQuota(e.Owner, quota.OpAgentQuery); err != nil {
		app.Log("events", "could not charge a scheduled run for %s: %v", e.Owner, err)
	}
	return answer
}
