package agent

import (
	"mu/internal/origin"
	"mu/service/events"
	"time"
)

// A brief, optional invitation. No model call, tracking or response is required.
func momentMessage(schedule *events.Event, at time.Time) string {
	loc, err := time.LoadLocation(schedule.Zone)
	if err != nil {
		loc = time.UTC
	}
	day := at.In(loc)
	messages := []string{
		"If it suits you, rest your gaze on one nearby object for ten seconds. Nothing needs to change.",
		"You can leave things unfinished. Pause where you are for thirty seconds, if you would like to.",
		"If it is comfortable for you, loosen your grip for a few seconds. That is enough.",
		"If it suits you, look away from the screen for ten seconds. There is nothing to complete afterwards.",
		"If it feels comfortable, rest one hand where it is for ten seconds. No need to move anywhere.",
		"If water is already within reach and drinking is comfortable for you, take one sip. Otherwise, let this invitation pass.",
		"If you would like to, listen to the sounds around you for ten seconds. You do not need to feel any particular way.",
	}
	index := int(time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).Unix()/86400) % len(messages)
	if index < 0 {
		index += len(messages)
	}
	return messages[index] + "\n\n[Manage Take a moment](" + origin.Self() + "/agents?view=scheduled#moment)"
}
