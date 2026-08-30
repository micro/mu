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
	"mu/internal/thread"
	"mu/service/tasks"
)

func TestAQuietAccountGetsNoBrief(t *testing.T) {
	const who = "brief-quiet"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	if got := briefHTML(who); got != "" {
		t.Errorf("an account with nothing happening got %q", got)
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

// It is a sentence, not a section: no rule and no heading, which is what
// separates it from the two labelled blocks either side.
func TestTheBriefIsNotASection(t *testing.T) {
	const who = "brief-shape"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck
	if _, err := tasks.Create(who, "Something", "", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}

	got := briefHTML(who)
	if !strings.HasPrefix(got, `<p class="home-brief">`) {
		t.Errorf("the brief is not a paragraph: %q", got)
	}
	if strings.Contains(got, "home-section") {
		t.Error("the brief drew itself a heading and a rule")
	}
}
