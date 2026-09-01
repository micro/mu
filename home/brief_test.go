package home

// How things are, in a sentence, and who else is here.
//
// What must hold is that the clauses say nothing when there is nothing to say.
// This is on the screen somebody sees most often, so a line reading "Nothing
// new" costs a glance every visit and gives nothing back.
//
// Who is here is no longer one of the clauses. It was, for a while, and it is
// its own block under the box now — see here_test.go. What is left here is the
// four clauses about your own day, and the rule that they say nothing when
// there is nothing to say.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/events"
	"mu/service/tasks"
)

// A quiet account gets nothing, and that includes being alone.
//
// "Nothing new" costs a glance and gives nothing back, on the screen somebody
// sees most often. For one commit this drew a section saying "Just you here"
// over a link to the chat, on the argument that who is present is true on the
// quietest day. True, and the two most useless sentences on the page: it told
// somebody they were alone and then invited them to go and talk about it. Who
// is here is a strip of names under the box now, which states the same fact
// without the sentence.
func TestAQuietAccountGetsNoBrief(t *testing.T) {
	const who = "brief-quiet"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	if got := briefHTML(who); got != "" {
		t.Errorf("an account with nothing happening, alone, got %q", got)
	}
	if got := briefHTML(""); got != "" {
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

	got := briefHTML(who)
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
	if got := briefHTML(who); strings.Contains(got, "waiting") {
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

	got := briefHTML(who)
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

// Labelled like the blocks under it, and labelled by the same thing that
// decides whether there is anything to label.
//
// It stood unlabelled while it was four counts this instance already held, on
// the argument that a heading over one line is furniture. It is a written
// sentence now, sitting immediately under a box you type into, and an
// unlabelled line there reads as output from the box rather than as a block of
// its own.
func TestTheBriefIsLabelledLikeEverythingElse(t *testing.T) {
	const who = "brief-shape"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck
	if _, err := tasks.Create(who, "Something", "", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}

	got := briefHTML(who)
	if !strings.HasPrefix(got, sectionRule("Brief")) {
		t.Errorf("the brief is not labelled: %q", got)
	}
	if !strings.Contains(got, `<p class="home-brief">`) {
		t.Errorf("the brief is not a paragraph: %q", got)
	}

	// Nothing here about the silent case: TestAQuietAccountGetsNoBrief pins
	// that, and it has to, because it runs before anything marks a second
	// person present.
}

// What is on today, and only what is still ahead.
//
// The brief said what was waiting, what the agent was doing and what was owed
// — three questions about work — and nothing about the day. Somebody with a
// dentist at four read a line about their inbox and then went to look at a
// calendar, which is the trip a brief exists to save.
func TestTheBriefSaysWhatIsOnToday(t *testing.T) {
	const who = "briefday"
	t.Cleanup(func() {
		for _, e := range events.List(who) {
			events.Remove(who, e.ID) //nolint:errcheck
		}
	})

	now := time.Now()
	soon := now.Add(2 * time.Hour)
	later := now.Add(4 * time.Hour)
	if soon.Day() != now.Day() || later.Day() != now.Day() {
		t.Skip("too near midnight for a same-day fixture to mean anything")
	}

	if _, err := events.Create(who, "Dentist", later, ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := events.Create(who, "School pickup", soon, ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	got := onToday(who)
	if got == "" {
		t.Fatal("two things in the diary today and the brief says nothing")
	}
	// The soonest, named, with its time — not merely the first one stored.
	if !strings.Contains(got, "School pickup") {
		t.Errorf("the next thing is not named, or is not the soonest: %s", got)
	}
	if !strings.Contains(got, soon.Format("15:04")) {
		t.Errorf("the time is missing, which is the fact somebody wants: %s", got)
	}
	if !strings.Contains(got, "1 more today") {
		t.Errorf("the rest of the day is not counted: %s", got)
	}
}

// An event that has already happened is not news.
//
// A brief read at six that says "3 things today" when all three are done is a
// number nobody can act on, and finding that out means opening the calendar.
func TestWhatHasAlreadyHappenedIsNotInTheBrief(t *testing.T) {
	const who = "briefpast"
	t.Cleanup(func() {
		for _, e := range events.List(who) {
			events.Remove(who, e.ID) //nolint:errcheck
		}
	})

	past := time.Now().Add(-2 * time.Hour)
	if past.Day() != time.Now().Day() {
		t.Skip("too near midnight for a same-day fixture to mean anything")
	}
	if _, err := events.Create(who, "Standup", past, ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	if got := onToday(who); got != "" {
		t.Errorf("an event that has already happened is still in the brief: %s", got)
	}
}

// And a diary that is empty says nothing at all, rather than saying it is empty.
func TestAnEmptyDiaryIsSilent(t *testing.T) {
	if got := onToday("briefnobody"); got != "" {
		t.Errorf("an account with no events gets a line about it: %s", got)
	}
}
