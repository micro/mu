package home

// The brief is about you. The world is about the services.
//
// The question that settled it: what use is a brief over the services, when
// some of that is useful and none of it is about you or your inbox.
//
// Two things were wrong in one. The world's sentence runs to 256 characters
// against clauses of about forty, so the block read as a news line with
// somebody's own day as a preamble — the proportions said what it was about
// whatever the order claimed. And the News card sits a few inches to the right
// showing the same stories, so it was a prose duplicate of the thing beside it.
//
// This pins the split, because the failure it guards against is somebody
// putting the world's sentence back into the personal block for the reason it
// was there the first time: on a quiet day it is the only line with anything in
// it. That is true and is an argument for a quiet page, not for filing the news
// under a heading about you.

import (
	"strings"
	"testing"
	"time"

	"mu/agent/brief"
	"mu/internal/auth"
	"mu/service/tasks"
)

// The personal brief carries no world news, even when there is some.
func TestTheBriefIsAboutYouAndNotTheWorld(t *testing.T) {
	const who = "briefsplit"
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck
	if _, err := tasks.Create(who, "Renew the domain", "", tasks.Me,
		time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// A line about the world exists and is being published.
	line := brief.Line()

	got := briefHTML(who)
	if got == "" {
		t.Fatal("an account with an overdue task gets no brief at all")
	}
	if !strings.Contains(got, "overdue") {
		t.Errorf("the brief does not say what is owed:\n%s", got)
	}
	if line != "" && strings.Contains(got, line) {
		t.Errorf("the world's sentence is inside the personal brief:\n%s", got)
	}
	// The block that carries it is the services', and it is not in the rail.
	if strings.Contains(got, "home-happening") {
		t.Errorf("the world's block is being drawn inside the brief:\n%s", got)
	}
}

// And the world's sentence is drawn, when there is one, in its own block.
//
// Guarded on there being a line at all: agent/brief writes on a timer and a
// test binary has not run it, so this checks the wiring rather than asserting
// that an instance always has news.
func TestTheWorldsSentenceIsItsOwnBlock(t *testing.T) {
	line := brief.Line()
	got := happening()

	if line == "" {
		if got != "" {
			t.Errorf("no line was written and something was drawn: %q", got)
		}
		return
	}
	if !strings.Contains(got, line) {
		t.Errorf("the world's block does not carry the line:\n%s", got)
	}
	if !strings.Contains(got, "home-happening") {
		t.Errorf("the world's block has no class of its own, so nothing can "+
			"set it apart from the cards under it:\n%s", got)
	}
}
