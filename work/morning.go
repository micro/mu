package work

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/origin"
	"mu/service/events"
	"mu/service/news"
	"mu/service/prayer"
	"mu/service/tasks"
	"mu/service/transit"
	"mu/service/weather"
)

type briefSource struct {
	Name string `json:"source"`
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

// Fetch a fixed set of read-only sources before the single included model call.
// In particular, a scheduled brief must not depend on someone having opened Home
// recently to warm its in-memory calendar cache.
func morningFacts(ctx context.Context, owner string, schedule *events.Event, now time.Time) (string, string) {
	acc, err := auth.GetAccount(owner)
	if err != nil {
		return "Account unavailable.", ""
	}
	loc, err := time.LoadLocation(schedule.Zone)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)
	days := 1
	if schedule.Repeat == "weekly" {
		days = 7
	}
	end := time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, loc)
	var b strings.Builder
	fmt.Fprintf(&b, "Brief date: %s. Name: %s. Saved location: %s.\n", now.Format(time.RFC1123), acc.Name, acc.Place)
	if days == 7 {
		b.WriteString("Prepare a weekly brief looking ahead seven days.\n")
	} else {
		b.WriteString("Prepare today's morning brief.\n")
	}
	if !events.BriefWorldNews(schedule) {
		b.WriteString("World news is disabled. Omit Headlines.\n")
	}
	if auth.Plan(owner) == "pro" && schedule.Plan {
		b.WriteString("A short suggested daily plan is explicitly requested. Base it on supplied deadlines and commitments; do not invent free time.\n")
	}

	type lookup struct {
		name string
		read func() briefSource
	}
	jobs := []lookup{{"Calendar", func() briefSource {
		entries := events.ExternalEvents(owner, now, end, 30)
		return briefSource{Name: "Calendar — visible records only; not proof of free time", Text: briefAgenda(events.Upcoming(owner), entries, now, end), URL: origin.Self() + "/events"}
	}}, {"Daily reminder", func() briefSource {
		d := prayer.DailyReminderForDate(now.Format("2006-01-02"))
		if d == nil || strings.TrimSpace(d.Verse) == "" {
			return briefSource{Name: "Daily reminder"}
		}
		// This source text is rendered verbatim after synthesis, never rewritten by a model.
		text := "## Daily reminder\n\n" + d.Verse
		if strings.TrimSpace(d.Message) != "" {
			text += "\n\n" + d.Message
		}
		text += "\n\n[Read the reminder](" + origin.Self() + "/prayer)"
		return briefSource{Name: "Daily reminder", Text: text}
	}}}
	havePlace := strings.TrimSpace(acc.Place) != "" || acc.Lat != 0 || acc.Lon != 0
	if havePlace && (acc.Lat != 0 || acc.Lon != 0) {
		jobs = append(jobs, lookup{"Weather", func() briefSource {
			return briefSource{Name: "Weather — saved location; use the provider's dated rows", Text: weather.ForecastText(ctx, acc.Lat, acc.Lon), URL: origin.Self() + "/weather"}
		}})
		jobs = append(jobs, lookup{"Prayer times", func() briefSource {
			var rsp prayer.TimesResponse
			err := (prayer.Server{}).Times(ctx, &prayer.TimesRequest{Lat: acc.Lat, Lon: acc.Lon, TZ: loc.String()}, &rsp)
			if err != nil {
				return briefSource{Name: "Prayer times", Text: "Today's calculation was unavailable."}
			}
			return briefSource{Name: "Prayer times — calculated with the service's default ISNA convention, not mosque congregation times", Text: rsp.Times, URL: origin.Self() + "/prayer"}
		}})
		// TfL covers London. Never label its result as local travel for other cities,
		// nor imply that it includes the user's National Rail route.
		if acc.Lat >= 51.28 && acc.Lat <= 51.70 && acc.Lon >= -0.52 && acc.Lon <= 0.34 {
			jobs = append(jobs, lookup{"London transport", func() briefSource {
				var rsp transit.StatusResponse
				if err := (transit.Server{}).Status(ctx, &transit.StatusRequest{}, &rsp); err != nil {
					return briefSource{Name: "London transport", Text: "TfL status was unavailable."}
				}
				return briefSource{Name: "London transport — TfL network only, not National Rail or a specific journey", Text: rsp.Text, URL: "https://tfl.gov.uk/tube-dlr-overground/status/"}
			}})
		}
	}
	results := make(chan briefSource, len(jobs))
	fetchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for _, job := range jobs {
		go func(job lookup) { results <- job.read() }(job)
	}
	sections := map[string]briefSource{}
	for remaining := len(jobs); remaining > 0; remaining-- {
		select {
		case result := <-results:
			sections[result.Name] = result
		case <-fetchCtx.Done():
			remaining = 0
		}
	}
	// Stable ordering despite parallel lookups; missing sources are not invented.
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	reflection := ""
	for _, k := range keys {
		src := sections[k]
		if src.Name == "Daily reminder" {
			reflection = src.Text
			continue
		}
		appendBriefSource(&b, src)
	}
	appendBriefSource(&b, briefSource{Name: "Relevant outstanding work", Text: briefWork(tasks.List(owner, ""), now, end), URL: origin.Self() + "/work"})
	if events.BriefWorldNews(schedule) {
		appendBriefSource(&b, briefSource{Name: "News published in the last 24 hours", Text: briefNews(news.GetFeed(), now)})
	}
	b.WriteString("\nOnly the sources supplied above are available. Missing calendar entries cannot establish availability. No inbox messages have been evaluated for whether a reply is owed.\n")
	return b.String(), reflection
}
func appendBriefSource(b *strings.Builder, src briefSource) {
	if strings.TrimSpace(src.Text) == "" {
		return
	}
	encoded, _ := json.Marshal(src)
	// Bound encoded bytes, preserving whole source lines (especially news URLs).
	for len(encoded) > 3500 {
		cut := strings.LastIndex(src.Text, "\n")
		if cut < 0 {
			src.Text = "Source omitted because it exceeded the brief size limit."
		} else {
			src.Text = src.Text[:cut]
		}
		encoded, _ = json.Marshal(src)
	}
	b.WriteString("\nSOURCE DATA: " + string(encoded) + "\n")
}
func briefAgenda(local []*events.Event, external []events.External, now, end time.Time) string {
	type item struct {
		at   time.Time
		text string
	}
	var rows []item
	for _, e := range local {
		if e == nil {
			continue
		}
		if e.Kind != "" || e.Prompt != "" || !e.When.Before(end) || e.When.Add(e.Length()).Before(now) {
			continue
		}
		rows = append(rows, item{e.When, e.When.In(now.Location()).Format("Mon 2 Jan 15:04") + " — " + e.Title})
	}
	for _, e := range external {
		if !e.Start.Before(end) || !e.End.After(now) {
			continue
		}
		label := e.Start.In(now.Location()).Format("Mon 2 Jan 15:04")
		if e.AllDay {
			label = e.Start.In(now.Location()).Format("Mon 2 Jan") + " (all day)"
		}
		rows = append(rows, item{e.Start, label + " — " + e.Title})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].at.Before(rows[j].at) })
	var lines []string
	seen := map[string]bool{}
	for _, r := range rows {
		if !seen[r.text] {
			seen[r.text] = true
			lines = append(lines, r.text)
		}
		if len(lines) == 20 {
			break
		}
	}
	if len(lines) == 0 {
		return "No calendar entries were returned for this window. This does not establish that the calendar is complete or that the person is free."
	}
	return strings.Join(lines, "\n")
}
func briefWork(items []*tasks.Task, now, end time.Time) string {
	var lines []string
	for _, t := range items {
		if t == nil || !t.Open() {
			continue
		}
		// Avoid repeatedly turning old failed/blocked jobs into a morning alarm.
		due := !t.Due.IsZero() && t.Due.Before(end)
		changed := !t.Updated.Before(now.Add(-24 * time.Hour))
		if !due && !changed {
			continue
		}
		line := fmt.Sprintf("%s [%s] (%s/work?id=%s)", t.Title, t.Status, origin.Self(), url.QueryEscape(t.ID))
		if due {
			line += "; due " + t.Due.In(now.Location()).Format("Mon 2 Jan 15:04")
		}
		if strings.TrimSpace(t.Result) != "" {
			reason := []rune(t.Result)
			if len(reason) > 300 {
				reason = reason[:300]
			}
			line += "; recorded outcome: " + string(reason)
		}
		lines = append(lines, line)
		if len(lines) == 5 {
			break
		}
	}
	return strings.Join(lines, "\n")
}
func briefNews(posts []*news.Post, now time.Time) string {
	recent := make([]*news.Post, 0, len(posts))
	for _, p := range posts {
		if p == nil || !hasPublicationDate(p.Published) || p.PostedAt.IsZero() || p.PostedAt.Before(now.Add(-24*time.Hour)) || p.PostedAt.After(now) || strings.TrimSpace(p.Title) == "" {
			continue
		}
		u, err := url.Parse(p.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			continue
		}
		recent = append(recent, p)
	}
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].PostedAt.After(recent[j].PostedAt) })
	var lines []string
	seen := map[string]bool{}
	categories := map[string]int{}
	for _, p := range recent {
		if seen[p.URL] || categories[p.Category] >= 3 {
			continue
		}
		seen[p.URL] = true
		categories[p.Category]++
		description := []rune(p.Description)
		if len(description) > 400 {
			description = description[:400]
		}
		line := fmt.Sprintf("Published %s: %s — %s\nSource: %s", p.PostedAt.In(now.Location()).Format(time.RFC3339), p.Title, string(description), p.URL)
		candidate, _ := json.Marshal(strings.Join(append(append([]string{}, lines...), line), "\n"))
		if len(candidate) > 3000 {
			continue
		}
		lines = append(lines, line)
		if len(lines) == 8 {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// PostedAt can be the ingestion time when RSS omitted or malformed its date.
// Such an item must not be passed off as freshly published in a morning brief.
func hasPublicationDate(value string) bool {
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, time.ANSIC, "2006-01-02T15:04:05-0700", "2006-01-02 15:04:05 -0700"} {
		if _, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return true
		}
	}
	return false
}
