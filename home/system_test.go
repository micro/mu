package home

import (
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/service/tasks"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-home-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// The home screen was seven cards of the world's content and nothing of yours.
// This is the other half: what you have and what is in flight.
func TestTheStripShowsYourOwnSystem(t *testing.T) {
	acc := &auth.Account{ID: "striper", Name: "striper", Secret: "s"}
	auth.Create(acc)

	tasks.Create(acc.ID, "Something to do", "", "", time.Time{})

	got := systemStrip(acc)
	for _, want := range []string{`href="/tasks"`, "Tasks", `href="/mail"`, "Unread",
		`href="/apps"`, "Apps", `href="/wallet"`, "Credits"} {
		if !strings.Contains(got, want) {
			t.Errorf("the strip is missing %q", want)
		}
	}
	// The task just created is counted.
	if !strings.Contains(got, `<span class="home-stat-n">1</span>`) {
		t.Errorf("an open task was not counted:\n%s", got)
	}
}

// A guest has no system to show, and the strip is not an advert.
func TestAGuestSeesNoStrip(t *testing.T) {
	if got := systemStrip(nil); got != "" {
		t.Errorf("a signed-out visitor was shown %q", got)
	}
}

// Zeroes are shown rather than hidden. A new account with nothing in it is
// still a shape, and a strip that only appeared once you had something would
// never be there when it was most useful.
func TestZeroesAreShownAndMarked(t *testing.T) {
	acc := &auth.Account{ID: "empty", Name: "empty", Secret: "s"}
	auth.Create(acc)

	got := systemStrip(acc)
	if !strings.Contains(got, "home-stat zero") {
		t.Errorf("an empty count was not marked as one:\n%s", got)
	}
	if strings.Count(got, "home-stat") < 4 {
		t.Errorf("an empty account lost entries from its strip:\n%s", got)
	}
}

// An admin is never charged, so a credit count means nothing to them — the same
// reason the top bar shows them the unlimited mark rather than a balance.
func TestAnAdminIsNotShownCredits(t *testing.T) {
	acc := &auth.Account{ID: "chief", Name: "chief", Secret: "s", Admin: true}
	auth.Create(acc)

	if got := systemStrip(acc); strings.Contains(got, "Credits") {
		t.Errorf("an admin was shown a credit balance:\n%s", got)
	}
}

// Work in flight is the thing you most want to see without opening anything.
func TestRunningWorkIsCalledOut(t *testing.T) {
	acc := &auth.Account{ID: "busy", Name: "busy", Secret: "s"}
	auth.Create(acc)

	task, _ := tasks.Create(acc.ID, "Slow thing", "", "agent", time.Time{})
	tasks.Update(acc.ID, task.ID, "", "", tasks.StatusDoing, "", "")

	if got := systemStrip(acc); !strings.Contains(got, "1 working") {
		t.Errorf("a task with the agent was not called out:\n%s", got)
	}
}
