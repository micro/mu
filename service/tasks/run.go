package tasks

// Handing a task to the agent.
//
// A list the agent can read is a to-do app. What makes this a service is that
// the work can be given away: mark a task for the agent, and it runs, and the
// answer comes back onto the task where it can be read later.
//
// The agent is reached through a hook main() sets, the same way events reaches
// it for a standing instruction — this package must not import the agent, and
// the agent must not know about tasks.

import (
	"fmt"
	"strings"
	"sync"

	"mu/internal/app"
	"mu/service/wallet"
)

// RunAgent is set by main() to run a prompt through the agent as an account.
var RunAgent func(accountID, prompt string) (string, error)

// running tracks the tasks currently with the agent, so a second Run on the
// same task does not start a second agent.
var (
	runMu   sync.Mutex
	running = map[string]bool{}
)

// Running reports whether the agent is working on a task right now.
func Running(id string) bool {
	runMu.Lock()
	defer runMu.Unlock()
	return running[id]
}

// Run hands a task to the agent and returns immediately.
//
// It returns immediately because an agent run takes seconds to a minute, and a
// request held open for that is a page that looks broken. The task moves to
// "doing" now and to "done" with its result when the agent finishes, so the
// list is the progress indicator.
func Run(owner, id string) error {
	if RunAgent == nil {
		return fmt.Errorf("the agent is not available on this instance")
	}
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if t.Status == StatusDone {
		return fmt.Errorf("that task is already done")
	}

	runMu.Lock()
	if running[t.ID] {
		runMu.Unlock()
		return fmt.Errorf("the agent is already working on that")
	}
	running[t.ID] = true
	runMu.Unlock()

	// An agent run is a model call, which is the one thing here that costs
	// real money. Check before starting rather than after: a task that cannot
	// be paid for should not move to "doing".
	canProceed, _, cost, err := wallet.CheckQuota(owner, wallet.OpAgentQuery)
	if err != nil || !canProceed {
		runMu.Lock()
		delete(running, t.ID)
		runMu.Unlock()
		return fmt.Errorf("this costs %d credits — top up at /wallet", cost)
	}

	if _, err := Update(owner, t.ID, "", "", StatusDoing, Agent, ""); err != nil {
		runMu.Lock()
		delete(running, t.ID)
		runMu.Unlock()
		return err
	}

	go func(t Task) {
		defer func() {
			runMu.Lock()
			delete(running, t.ID)
			runMu.Unlock()
		}()

		answer, err := RunAgent(t.Owner, prompt(t))
		if err != nil {
			// The task stays open: a failed run is work still to do, not work
			// finished badly, and the reason belongs where someone will see it.
			app.Log("tasks", "task %q failed for %s: %v", t.Title, t.Owner, err)
			Update(t.Owner, t.ID, "", "", StatusTodo, "", "Last run failed: "+err.Error()) //nolint:errcheck
			return
		}
		if err := wallet.ConsumeQuota(t.Owner, wallet.OpAgentQuery); err != nil {
			app.Log("tasks", "could not charge task run for %s: %v", t.Owner, err)
		}
		Update(t.Owner, t.ID, "", "", StatusDone, "", strings.TrimSpace(answer)) //nolint:errcheck
	}(*t)

	return nil
}

// prompt is what the agent is actually asked. The title is the instruction and
// the detail is the context; saying so beats hoping the model infers it.
func prompt(t Task) string {
	if t.Detail == "" {
		return t.Title
	}
	return t.Title + "\n\n" + t.Detail
}
