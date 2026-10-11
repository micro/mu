package agent

import (
	"fmt"
	"mu/internal/origin"
	"mu/service/events"
	"mu/service/prayer"
	"strings"
	"time"
)

func eveningReflection(e *events.Event, at time.Time) (string, error) {
	loc, err := time.LoadLocation(e.Zone)
	if err != nil {
		loc = time.UTC
	}
	date := at.In(loc).Format("2006-01-02")
	return renderEveningReflection(prayer.DailyReminderForDate(date), date)
}

// The reminder already contains the reflection. Preserve its wording rather
// than asking a second model to reinterpret it or generate scripture.
func renderEveningReflection(reminder *prayer.ReminderData, date string) (string, error) {
	if reminder == nil || strings.TrimSpace(reminder.Message) == "" {
		return "", fmt.Errorf("the daily reminder is unavailable for %s", date)
	}
	return strings.TrimSpace(reminder.Message) + "\n\nIf you would like, take a quiet moment to consider what this brings to mind from your day. No reply is needed.\n\n[Daily reminder](https://reminder.dev/api/daily?date=" + date + ")\n\n[Manage evening reflection](" + origin.Self() + "/agents?view=scheduled&event=research)", nil
}
