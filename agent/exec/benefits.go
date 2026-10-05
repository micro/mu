package exec

import (
	"mu/agent"
	"mu/service/events"
)

func benefitRun(r request) bool {
	if r.Kind != events.Kind {
		return false
	}
	handled, answer, err := agent.RunScheduled(r.Account, r.ID, r.Revision, r.Due)
	if handled && (answer != "" || err != nil) {
		deliver(r, answer, err)
	}
	return handled
}
