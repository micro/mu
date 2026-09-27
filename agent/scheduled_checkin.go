package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/origin"
	"mu/service/events"
)

// The invitation needs no model call or tools. Planning starts only on reply.
func checkinMessage(owner string, schedule *events.Event, now time.Time) string {
	loc, err := time.LoadLocation(schedule.Zone)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)
	end := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	greeting := "Morning"
	if now.Hour() >= 12 {
		greeting = "Hello"
	}
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil && strings.TrimSpace(acc.Name) != "" {
		greeting += " " + checkinText(acc.Name)
	}
	var b strings.Builder
	b.WriteString("## Daily Checkin\n\n" + now.Format("Monday, 2 January 2006") + "\n\n" + greeting + ". How’s it going?\n\n## Today\n")
	type commitment struct {
		when   time.Time
		title  string
		allDay bool
	}
	var agenda []commitment
	for _, e := range events.Upcoming(owner) {
		if e.Kind == "" && e.Prompt == "" && !e.When.Before(now) && e.When.Before(end) {
			agenda = append(agenda, commitment{e.When, e.Title, false})
		}
	}
	for _, e := range events.ExternalEvents(owner, now, end, 10) {
		if e.Start.Before(end) && e.End.After(now) {
			agenda = append(agenda, commitment{e.Start, e.Title, e.AllDay})
		}
	}
	sort.Slice(agenda, func(i, j int) bool { return agenda[i].when.Before(agenda[j].when) })
	if len(agenda) > 0 {
		b.WriteString("\nFrom your calendar:\n")
		for i, e := range agenda {
			if i == 3 {
				break
			}
			label := e.when.In(loc).Format("15:04")
			if e.allDay {
				label = "All day"
			}
			fmt.Fprintf(&b, "- %s — %s\n", label, checkinText(e.title))
		}
	}
	if len(agenda) == 0 {
		b.WriteString("\nNo upcoming calendar entries were returned for today.\n")
	}
	b.WriteString("\n## Your focus\n\nWhat do you need to get done, or need help with? One or two sentences is enough.\n\nReply when you’re ready. We can choose a priority and work out the next step together.\n\n[Manage your check-in](" + origin.Self() + "/agents?view=scheduled#checkin)")
	return b.String()
}

func checkinText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "\\", "\\\\").Replace(s)
}
