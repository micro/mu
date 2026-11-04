package agent

import (
	"fmt"
	"regexp"

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

// Imported reflections use section headings, never a second document title.
var reminderTitle = regexp.MustCompile(`(?m)^ {0,3}#([[:blank:]]+)`)

func renderMorningBrief(c BriefContent, name string, e *events.Event, now time.Time, reminder string, plan bool) string {
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
	var sources []string
	seenSources := map[string]bool{}
	section := func(title string, items []BriefItem, limit int, fallback string) {
		count := 0
		for _, item := range items {
			text := briefText(item.Text)
			if text == "" {
				continue
			}
			if count == 0 {
				fmt.Fprintf(&b, "\n## %s\n\n", title)
			}
			u, err := url.Parse(item.URL)
			if err == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") && !strings.ContainsAny(item.URL, "\n\r<>") {
				if !seenSources[u.String()] {
					seenSources[u.String()] = true
					label := title
					if item.Title != "" {
						label = item.Title
					}
					sources = append(sources, "["+briefText(label+" · "+u.Hostname())+"](<"+u.String()+">)")
				}
			}
			if label := briefText(item.Title); label != "" {
				text = "**" + label + "** — " + text
			} else if title == "Headlines" {
				text = "**" + text + "**"
			}
			fmt.Fprintf(&b, "- %s\n", text)
			count++
			if count == limit {
				break
			}
		}
		if count == 0 && fallback != "" {
			fmt.Fprintf(&b, "\n## %s\n\n", title)
			b.WriteString(fallback + "\n")
		}
	}
	section("Your day", c.Day, 8, "No calendar items were available. This may not include all your commitments.")
	section("Weather", c.Weather, 1, "")
	if len(c.Travel) > 0 {
		section("Travel", c.Travel, 3, "")
	}
	section("Prayer times", c.Prayer, 1, "")
	if BriefWorldNews(e) {
		section("Headlines", c.Headlines, 5, "")
	}
	section("Markets", c.Markets, 8, "")
	if plan && len(c.Priorities) > 0 {
		section("One small step", c.Priorities, 1, "")
	}
	if reminder != "" {
		b.WriteString("\n" + reminderTitle.ReplaceAllString(reminder, "##$1") + "\n")
	} else {
		b.WriteString("\n## Daily reminder\n\nToday's verse is unavailable.\n")
	}
	if len(sources) > 0 {
		b.WriteString("\nSources: " + strings.Join(sources, " · ") + "\n")
	}
	return strings.TrimSpace(b.String())
}
