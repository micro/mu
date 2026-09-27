package work

import (
	"fmt"
	"mu/agent"
	"mu/service/events"
	"net/url"
	"strings"
	"time"
)

// The application owns headings, ordering, fallbacks and optional sections.
// Model strings are plain text: they cannot introduce headings or links.
func briefText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "#", "\\#", "<", "&lt;", ">", "&gt;", "`", "\\`", "!", "\\!", "|", "\\|").Replace(s)
}
func renderMorningBrief(c agent.BriefContent, name string, e *events.Event, now time.Time, reminder string, plan bool) string {
	loc, err := time.LoadLocation(e.Zone)
	if err != nil {
		loc = time.UTC
	}
	greeting := "Good morning."
	if strings.TrimSpace(name) != "" {
		greeting = "Good morning, " + briefText(name) + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s\n", greeting, now.In(loc).Format("Monday, 2 January 2006"))
	section := func(title string, items []agent.BriefItem, limit int, fallback string) {
		fmt.Fprintf(&b, "\n## %s\n\n", title)
		count := 0
		for _, item := range items {
			text := briefText(item.Text)
			if text == "" {
				continue
			}
			u, err := url.Parse(item.URL)
			if err == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") && !strings.ContainsAny(item.URL, "\n\r<>") {
				text += " [Source](<" + u.String() + ">)"
			}
			fmt.Fprintf(&b, "- %s\n", text)
			count++
			if count == limit {
				break
			}
		}
		if count == 0 {
			b.WriteString(fallback + "\n")
		}
	}
	section("Your day", c.Day, 8, "No commitments or relevant work were available for this brief. Calendar coverage may be incomplete.")
	section("Weather", c.Weather, 1, "Today's weather is unavailable.")
	section("Prayer times", c.Prayer, 1, "Today's prayer times are unavailable.")
	if events.BriefWorldNews(e) {
		section("Headlines", c.Headlines, 5, "No recent sourced headlines are available.")
	}
	if plan && len(c.Priorities) > 0 {
		section("Suggested priorities", c.Priorities, 3, "No priorities were suggested.")
	}
	if reminder != "" {
		b.WriteString("\n" + reminder + "\n")
	} else {
		b.WriteString("\n## Daily reminder\n\nToday's reminder is unavailable.\n")
	}
	return strings.TrimSpace(b.String())
}
