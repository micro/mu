package agent

import (
	"mu/service/events"
	"mu/service/news"
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestBriefMissingCalendarDoesNotEstablishAvailability(t *testing.T) {
	now := time.Date(2026, 9, 27, 6, 0, 0, 0, time.UTC)
	text := briefAgenda(nil, nil, now, now.Add(18*time.Hour))
	if !strings.Contains(text, "does not establish") {
		t.Fatal(text)
	}
	text = briefAgenda(nil, []events.External{{Title: "Tomorrow", Start: now.Add(25 * time.Hour), End: now.Add(26 * time.Hour)}, {Title: "Breakfast", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}}, now, now.Add(18*time.Hour))
	if strings.Contains(text, "Tomorrow") || !strings.Contains(text, "Breakfast") {
		t.Fatal(text)
	}
}
func TestBriefWorkOmitsOldBlockers(t *testing.T) {
	now := time.Now()
	text := briefWork([]*tasks.Task{{ID: "old", Title: "Old blocked job", Status: "blocked", Updated: now.Add(-72 * time.Hour)}, {ID: "due", Title: "Submit report", Status: "blocked", Due: now.Add(time.Hour), Result: "Needs the invoice"}}, now, now.Add(18*time.Hour))
	if strings.Contains(text, "Old blocked") || !strings.Contains(text, "Submit report") || !strings.Contains(text, "Needs the invoice") || !strings.Contains(text, "/work?id=due") {
		t.Fatal(text)
	}
}
func TestBriefNewsRequiresDatedLinkedRecentSources(t *testing.T) {
	now := time.Now()
	valid := &news.Post{Title: "Recent", URL: "https://example.com/recent", PostedAt: now.Add(-time.Hour), Published: now.Add(-time.Hour).Format(time.RFC3339)}
	text := briefNews([]*news.Post{valid, valid, {Title: "Old", URL: "https://example.com/old", PostedAt: now.Add(-48 * time.Hour), Published: now.Add(-48 * time.Hour).Format(time.RFC3339)}, {Title: "Undated", URL: "https://example.com/undated", PostedAt: now}}, now)
	if strings.Count(text, valid.URL) != 1 || strings.Contains(text, "Old") || strings.Contains(text, "Undated") {
		t.Fatal(text)
	}
}
