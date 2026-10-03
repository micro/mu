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
		"A little space in the day. Step outside for a few minutes, if that suits you. There is nothing you need to accomplish while you’re there.",
		"You can leave things unfinished for a moment. Settle somewhere comfortable and let yourself pause.",
		"If it feels comfortable, change position or have a gentle stretch. Resting is an option too.",
		"Take a moment away from the screen, if you can. Notice something around you without needing to do anything about it.",
		"A short walk might feel good, if that is available to you. Sitting somewhere different can offer a pause too.",
		"A moment to check what you need: perhaps some water, a little quiet or a rest. Choose what suits you.",
		"You don’t need to resolve everything today. There is room for a small pause.",
	}
	index := int(time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).Unix()/86400) % len(messages)
	if index < 0 {
		index += len(messages)
	}
	return messages[index] + "\n\n[Manage Take a moment](" + origin.Self() + "/agents?view=scheduled#moment)"
}
