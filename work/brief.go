package work

import "mu/service/events"

func scheduledBrief(r request) *events.Event {
	for _, e := range events.List(r.Account) {
		if e.ID == r.ID && e.Kind == "brief" {
			return e
		}
	}
	return nil
}
