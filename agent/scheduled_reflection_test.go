package agent

import (
	"mu/internal/auth"
	"mu/service/blog"
	"mu/service/events"
	"strings"
	"testing"
	"time"
)

func TestEveningDigestMigrationPreservesConsentAndTime(t *testing.T) {
	const owner = "reflection-migration"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	defer events.DeleteAll(owner)
	when := time.Now().Add(time.Hour)
	for _, paused := range []bool{false, true} {
		old := events.Event{ID: owner, Owner: owner, Kind: "research", Title: "Evening Reading: Islam", Prompt: "Islam", Paused: paused, When: when, Zone: "Europe/London", Repeat: "daily", Sequence: 7, MaxCredits: 6}
		if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[old.ID] = &old; return nil }); err != nil {
			t.Fatal(err)
		}
		configureScheduledAdvances()
		got := Research(owner)
		if got.Title != "Evening Digest" || got.Prompt != "Islam" || got.Paused != paused || !got.When.Equal(when) || got.Zone != old.Zone || got.Repeat != old.Repeat || got.Sequence != 8 || got.MaxCredits != 6 {
			t.Fatalf("migration changed schedule: %+v", got)
		}
		configureScheduledAdvances()
		if Research(owner).Sequence != 8 {
			t.Fatal("migration was not idempotent")
		}
	}
}

func TestEveningDigestUsesFreshPublicTopicPosts(t *testing.T) {
	at := time.Date(2026, 10, 13, 20, 30, 0, 0, time.UTC)
	post := func(id, tags string, age time.Duration) *blog.Post {
		return &blog.Post{ID: id, AuthorID: auth.MicroID, Title: id, Tags: tags, Content: "First paragraph for " + id + ".\n\nSecond paragraph.", CreatedAt: at.Add(-age)}
	}
	tech := post("tech", "opinion,tech", time.Hour)
	world := post("world", "digest", time.Hour)
	private := post("private", "opinion,tech", time.Minute)
	private.Private = true
	foreign := post("foreign", "opinion,tech", time.Minute)
	foreign.AuthorID = "other"
	posts := []*blog.Post{post("stale", "opinion,tech", 25*time.Hour), tech, world, private, foreign, post("future", "opinion,tech", -time.Hour), post("older-tech", "opinion,tech", 2*time.Hour), post("finance", "opinion,finance", time.Hour)}
	text, err := renderEveningDigest(posts, "", at)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"First paragraph for tech.", "id=tech", "id=world", "id=finance"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %s", want, text)
		}
	}
	for _, bad := range []string{"private", "foreign", "stale", "future", "older-tech"} {
		if strings.Contains(text, bad) {
			t.Errorf("leaked %s", bad)
		}
	}
	text, err = renderEveningDigest(posts, "Tech", at)
	if err != nil || !strings.Contains(text, "id=tech") || strings.Contains(text, "id=world") || strings.Contains(text, "id=finance") || strings.Contains(text, "Second paragraph") {
		t.Fatalf("topic digest: %s %v", text, err)
	}
	text, err = renderEveningDigest(posts, "Islam", at)
	if err != nil || !strings.Contains(text, "No new Islam digests") {
		t.Fatalf("empty topic: %s %v", text, err)
	}
}

func TestDigestExcerptDoesNotCutMarkdownLinks(t *testing.T) {
	content := strings.Repeat("word ", 87) + "[Dow rose 500 points](https://www.cnbc.com/2026/10/16/markets.html) after trading."
	excerpt := digestParagraph(content)
	if strings.Contains(excerpt, "https:") || strings.Contains(excerpt, "[Dow") || !strings.Contains(excerpt, "Dow rose 500") {
		t.Fatalf("broken link in excerpt: %s", excerpt)
	}
}

func TestEveningDigestUsesLocalCalendarDayAndLabelsRoundup(t *testing.T) {
	zone, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 16, 20, 0, 0, 0, zone)
	posts := []*blog.Post{
		{ID: "yesterday", AuthorID: auth.MicroID, Tags: "opinion,tech", Content: "Yesterday only.", CreatedAt: time.Date(2026, 10, 15, 23, 30, 0, 0, zone)},
		{ID: "morning", AuthorID: auth.MicroID, Tags: "digest", Content: "Thursday's trading ended lower.", CreatedAt: time.Date(2026, 10, 16, 6, 0, 0, 0, zone)},
	}
	result, err := renderEveningDigest(posts, "", at)
	if err != nil || strings.Contains(result, "Yesterday only") || !strings.Contains(result, "Published Friday 16 October, 06:00 BST") || !strings.Contains(result, "Thursday’s trading") {
		t.Fatalf("incorrect date handling: %s (%v)", result, err)
	}
}

func TestBreakRenamePreservesSchedule(t *testing.T) {
	const owner = "break-rename"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	defer events.DeleteAll(owner)
	when := time.Now().Add(time.Hour)
	old := events.Event{ID: "break", Owner: owner, Kind: "moment", Title: "Take a moment", When: when, Zone: "Europe/London", Repeat: "weekdays", Paused: true, Sequence: 7, Advance: scheduledAdvance("moment")}
	if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[old.ID] = &old; return nil }); err != nil {
		t.Fatal(err)
	}
	configureScheduledAdvances()
	got := Moment(owner)
	if got.Title != "Take a break" || !got.When.Equal(when) || !got.Paused || got.Zone != old.Zone || got.Repeat != old.Repeat || got.Sequence != 8 {
		t.Fatalf("changed schedule: %+v", got)
	}
	configureScheduledAdvances()
	if Moment(owner).Sequence != 8 {
		t.Fatal("rename repeated")
	}
}
