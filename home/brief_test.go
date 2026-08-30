package home

// How things are, in a sentence.
//
// The thing that must hold is that it says nothing when there is nothing to
// say. It sits on the screen somebody sees most often, so a line reading
// "Nothing new" costs a glance every visit and gives nothing back — and a
// section that is always there stops being read.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/thread"
	"mu/service/tasks"
)

func TestAQuietAccountGetsNoBrief(t *testing.T) {
	const who = "brief-quiet"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	if got := brief(who); got != "" {
		t.Errorf("an account with nothing happening got %q", got)
	}
	if got := brief(""); got != "" {
		t.Errorf("a signed-out reader got %q", got)
	}
}

// What arrived, said with who it is from — because "3 waiting" is a number and
// "3 waiting, the newest from Henrik" is a reason to open it or not.
func TestTheBriefSaysWhatArrivedAndWhoFrom(t *testing.T) {
	const who = "brief-arrived"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	th := thread.Open(who, "mail", "<brief@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Join(who, th.ID, thread.Party{Kind: thread.RolePerson,
		Key: "henrik@example.com", Name: "Henrik"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Are you free Tuesday?", From: "henrik@example.com"})

	got := brief(who)
	if !strings.Contains(got, "1 conversation") {
		t.Errorf("the brief does not count what arrived:\n%s", got)
	}
	if !strings.Contains(got, "Henrik") {
		t.Errorf("the brief does not say who it is from:\n%s", got)
	}
	if !strings.Contains(got, `href="/inbox"`) {
		t.Errorf("the count is not a way in:\n%s", got)
	}

	// Read, and it stops being news.
	thread.MarkSeen(who, th.ID)
	if got := brief(who); strings.Contains(got, "waiting") {
		t.Errorf("a conversation that has been read is still reported:\n%s", got)
	}
}

// Work in hand, and work owed. Overdue is called out on its own, because it is
// the one fact here somebody may have to act on today.
func TestTheBriefSeparatesWorkInHandFromWorkOwed(t *testing.T) {
	const who = "brief-tasks"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	running, err := tasks.Create(who, "Summarise the quarter", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Update(who, running.ID, "", "", tasks.StatusDoing, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(who, "Renew the domain", "", tasks.Me,
		time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got := brief(who)
	if !strings.Contains(got, "The agent is on") || !strings.Contains(got, "1 thing") {
		t.Errorf("work in hand is not reported:\n%s", got)
	}
	if !strings.Contains(got, "1 task") || !strings.Contains(got, "overdue") {
		t.Errorf("an overdue task is not called out:\n%s", got)
	}
	if !strings.Contains(got, `href="/tasks"`) {
		t.Errorf("the counts are not a way in:\n%s", got)
	}
}

// It is a sentence, not a section: no rule and no heading, which is what
// separates it from the two labelled blocks either side.
func TestTheBriefIsNotASection(t *testing.T) {
	const who = "brief-shape"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck
	if _, err := tasks.Create(who, "Something", "", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}

	got := brief(who)
	if !strings.HasPrefix(got, `<p class="home-brief">`) {
		t.Errorf("the brief is not a paragraph: %q", got)
	}
	if strings.Contains(got, "home-section") {
		t.Error("the brief drew itself a heading and a rule")
	}
}

// What came in today, which is the one clause that is not about you.
//
// The page these sit on shows sixteen live cards below two sections that only
// change when you use them, so an instance fetching all day looked idle from
// the front. This is the line that says it did not.
func TestTheBriefSaysWhatCameInToday(t *testing.T) {
	const who = "brief-world"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	now := time.Now()
	data.StartIndexing()
	for _, e := range []struct {
		id, kind, title string
		at              time.Time
	}{
		{"bw-n1", "news", "Rates held at 4%", now.Add(-3 * time.Hour)},
		{"bw-n2", "news", "A quiet week for equities", now.Add(-2 * time.Hour)},
		{"bw-v1", "video", "Explaining interest rates", now.Add(-30 * time.Minute)},
		{"bw-old", "news", "Something from March", now.AddDate(0, 0, -60)},
	} {
		data.Index(e.id, e.kind, e.title, "body", map[string]interface{}{
			"posted_at": e.at.Format(time.RFC3339),
		})
	}
	for _, id := range []string{"bw-n1", "bw-n2", "bw-v1", "bw-old"} {
		deadline := time.Now().Add(5 * time.Second)
		for data.ByID(id) == nil {
			if time.Now().After(deadline) {
				t.Fatalf("%s was never indexed", id)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	got := brief(who)
	if !strings.Contains(got, "2 stories and 1 video") {
		t.Errorf("what came in is not counted in words:\n%s", got)
	}
	if !strings.Contains(got, "today") {
		t.Errorf("the clause does not say when:\n%s", got)
	}
	if !strings.Contains(got, `href="/archive"`) {
		t.Errorf("the count is not a way in:\n%s", got)
	}
	if !strings.Contains(got, "Explaining interest rates") {
		t.Errorf("the newest thing is not named:\n%s", got)
	}
	if strings.Contains(got, "Something from March") {
		t.Errorf("a two month old article was called the newest:\n%s", got)
	}
}

// A list is written the way it is said.
func TestASeriesReadsLikeASentence(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"9 stories"}, "9 stories"},
		{[]string{"9 stories", "1 video"}, "9 stories and 1 video"},
		{[]string{"9 stories", "3 videos", "2 posts"}, "9 stories, 3 videos and 2 posts"},
	} {
		if got := series(c.in); got != c.want {
			t.Errorf("series(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Headlines run long, and this one is quoted inside a clause about three other
// things. Cut at a word, and count letters rather than bytes.
func TestALongHeadlineIsCutAtAWord(t *testing.T) {
	const long = "Central bank holds rates steady as inflation cools faster than forecast"

	got := clip(long, 40)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a cut headline does not say it was cut: %q", got)
	}
	if len([]rune(got)) > 41 {
		t.Errorf("clip did not cut: %q", got)
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Errorf("the ellipsis follows a space: %q", got)
	}
	if !strings.HasPrefix(long, strings.TrimSuffix(got, "…")) {
		t.Errorf("clip changed the words: %q", got)
	}

	if got := clip("Rates held", 40); got != "Rates held" {
		t.Errorf("a short headline was touched: %q", got)
	}

	// Runes, not bytes. Arabic is two bytes a letter, so a limit read as bytes
	// keeps half the title and can cut through a character. The cut has to be
	// a prefix of the original, which a mid-character one is not.
	const arabic = "بسم الله الرحمن الرحيم والحمد لله رب العالمين"
	cut := strings.TrimSuffix(clip(arabic, 40), "…")
	if !strings.HasPrefix(arabic, cut) {
		t.Errorf("clip cut through a character: %q", cut)
	}
	if n := len([]rune(cut)); n < 30 {
		t.Errorf("clip kept %d letters of a 40 letter budget — it is counting bytes", n)
	}
}
