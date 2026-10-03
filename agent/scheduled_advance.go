package agent

import (
	"context"
	"fmt"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/events"
	"time"
)

func scheduledAdvance(kind string) events.Advance {
	minutes := 0
	switch kind {
	case "brief":
		minutes = 10
	case "checkin", "moment":
		minutes = 1
	case "research":
		minutes = 15
	}
	if minutes == 0 {
		return events.Advance{}
	}
	return events.Advance{Minutes: minutes, Recipient: "agent"}
}

func configureScheduledAdvances() {
	for _, acc := range auth.AllAccounts() {
		needed := false
		for _, e := range events.List(acc.ID) {
			if e.Advance.Minutes == 0 && scheduledAdvance(e.Kind).Minutes > 0 {
				needed = true
				break
			}
		}
		if !needed {
			continue
		}
		if err := events.EditOwned(acc.ID, func(records map[string]*events.Event) error {
			for _, e := range records {
				if e.Advance.Minutes == 0 {
					e.Advance = scheduledAdvance(e.Kind)
				}
			}
			return nil
		}); err != nil {
			app.Log("agent", "configure scheduled preparation: %v", err)
		}
	}
}

// ScheduledCurrent checks ownership and revision again before releasing a result.
func ScheduledCurrent(owner, id, revision string) bool {
	acc, err := auth.GetAccount(owner)
	if err != nil || acc.Banned {
		return false
	}
	for _, e := range events.List(owner) {
		if e.ID == id {
			return !e.Paused && fmt.Sprint(e.Sequence) == revision && (e.Kind != "research" || auth.Plan(owner) == "pro")
		}
	}
	return false
}

// PrepareScheduled only permits read-only tools for custom advance preparation.
func PrepareScheduled(owner, id, revision string, due time.Time) (bool, string, error) {
	handled, answer, err := RunScheduled(owner, id, revision, due)
	if handled || err != nil {
		return handled, answer, err
	}
	if !ScheduledCurrent(owner, id, revision) {
		return true, "", nil
	}
	for _, e := range events.List(owner) {
		if e.ID != id {
			continue
		}
		var allowed []string
		for _, spec := range service.Specs() {
			for method, ep := range spec.Endpoints {
				if !ep.Writes && !ep.Destructive {
					allowed = append(allowed, spec.Tool(method))
				}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		answer, err := QueryWithOpts(owner, e.Prompt, QueryOpts{RunContext: ctx, RawReply: true, Tools: allowed, NoTools: len(allowed) == 0, System: "Prepare the requested reading or report for delivery at " + due.Format(time.RFC3339) + ". Only gather information and draft the result. Do not send messages or change records. Explain any requested action that cannot be performed during preparation."})
		return true, answer, err
	}
	return true, "", nil
}
