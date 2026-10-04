package agent

import (
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/origin"
	"mu/service/events"
)

// The invitation needs no model call or tools. The person sets the direction.
func checkinMessage(owner string, schedule *events.Event, now time.Time) string {
	loc, err := time.LoadLocation(schedule.Zone)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)
	greeting := "Morning"
	if now.Hour() >= 12 {
		greeting = "Hello"
	}
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil && strings.TrimSpace(acc.Name) != "" {
		greeting += " " + checkinText(acc.Name)
	}
	return greeting + ". How's it going?\n\n[Manage your check-in](" + origin.Self() + "/agents?view=scheduled#checkin)"
}

func checkinText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "\\", "\\\\").Replace(s)
}
