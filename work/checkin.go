package work

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/origin"
	"mu/service/events"
	"mu/service/tasks"
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
	b.WriteString(greeting + ". What would you like to focus on today?\n")
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
	for _, e := range events.CachedOverview(owner) {
		if e.Start.Before(end) && e.End.After(now) {
			agenda = append(agenda, commitment{e.Start, e.Title, e.AllDay})
		}
	}
	sort.Slice(agenda, func(i, j int) bool { return agenda[i].when.Before(agenda[j].when) })
	if len(agenda) > 0 {
		b.WriteString("\nToday, from your saved calendar information:\n")
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
	count := 0
	for _, t := range tasks.List(owner, "") {
		if t.Archived || t.Status == tasks.StatusDone || t.Status == tasks.StatusCanceled {
			continue
		}
		if count == 0 {
			b.WriteString("\nA few outstanding tasks:\n")
		}
		b.WriteString("- " + checkinText(t.Title) + "\n")
		count++
		if count == 3 {
			break
		}
	}
	b.WriteString("\nOne or two sentences is enough: tell me what you need to get done or need help with. I can help you choose a priority and work out the next steps. Nothing has been changed.\n\nReply whenever you are ready. There are no follow-up nudges.\n\n[Manage your check-in](" + origin.Self() + "/events?view=brief#checkin)")
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
