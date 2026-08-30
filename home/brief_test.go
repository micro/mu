package home

// How things are, in a sentence, and who else is here.
//
// What must hold is that the clauses say nothing when there is nothing to say.
// This is on the screen somebody sees most often, so a line reading "Nothing
// new" costs a glance every visit and gives nothing back.
//
// The section around them is always drawn, which is the one thing that is
// deliberately not silent: who is online is true on the quietest day, and the
// place you find that out cannot be a block that only appears when the news is
// good.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/tasks"
)

// A quiet account gets the room, and none of the clauses.
//
// The clauses stay silent — "Nothing new" costs a glance and gives nothing
// back. The section does not, because who else is here is true whether or not
// the news is interesting, and a line that only appears on a busy day cannot
// be where somebody finds out a friend is online.
func TestAQuietAccountGetsTheRoomAndNoClauses(t *testing.T) {
	const who = "brief-quiet"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	got := briefHTML(who)
	if strings.Contains(got, "home-brief") {
		t.Errorf("an account with nothing happening got a clause:\n%s", got)
	}
	if !strings.Contains(got, "home-here") {
		t.Errorf("the brief does not say who is here:\n%s", got)
	}
	if !strings.Contains(got, `href="/chat"`) {
		t.Errorf("there is no way through to the chat:\n%s", got)
	}

	// Signed out there is no page to put it on.
	if got := briefHTML(""); got != "" {
		t.Errorf("a signed-out reader got %q", got)
	}
}

// Who is here, counted. One person is you, and saying "1 person online" about
// yourself reads as a fault rather than as quiet.
func TestWhoIsHereIsCounted(t *testing.T) {
	if got := here(); !strings.Contains(got, "Just you here") {
		t.Errorf("alone, the brief says %q", got)
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

	// The clauses can be empty and the section still stands, because the two
	// lines under them are true on the quietest day there is.
	quiet := briefHTML("brief-shape-silent")
	if !strings.HasPrefix(quiet, sectionRule("Brief")) {
		t.Errorf("a quiet brief lost its heading: %q", quiet)
	}
	if strings.Contains(quiet, "home-brief") {
		t.Errorf("a quiet brief drew a clause: %q", quiet)
	}
}
