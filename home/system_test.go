package home

import (
	"html"
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

	// Absorb the admin bootstrap. auth.Create promotes the first account on a
	// fresh instance to admin, and admins are shown "unlimited" rather than a
	// credit count — so whichever test happened to run first got an account
	// that silently behaved differently from the one it was written about.
	// Claiming it here makes every test below an ordinary user, whatever order
	// they run in.
	auth.Create(&auth.Account{ID: "bootstrap", Name: "bootstrap", Secret: "s"})

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// The home screen was seven cards of the world's content and nothing of yours.
// This is the other half: what you have and what is in flight.
func TestTheStripShowsYourOwnSystem(t *testing.T) {
	acc := &auth.Account{ID: "striper", Name: "striper", Secret: "s"}
	auth.Create(acc)

	got := systemStrip(acc)
	for _, want := range []string{`href="/agents"`, "Agents", `href="/mail"`, "Unread",
		`href="/apps"`, "Apps", `href="/wallet"`, "Credits"} {
		if !strings.Contains(got, want) {
			t.Errorf("the strip is missing %q", want)
		}
	}
}

// Agents leads the strip, not Tasks.
//
// Tasks led it because the strip was written for a personal home server, where
// a to-do list is what you keep. For tools for agents it is the wrong noun, and
// it was a number that stayed at zero because nobody assigns themselves tasks
// here — the least useful thing that can lead a dashboard.
func TestTheStripLeadsWithAgentsNotTasks(t *testing.T) {
	acc := &auth.Account{ID: "leader", Name: "leader", Secret: "s"}
	auth.Create(acc)

	got := systemStrip(acc)
	if i, j := strings.Index(got, "Agents"), strings.Index(got, "Unread"); i < 0 || i > j {
		t.Errorf("Agents is not the first tile:\n%s", got)
	}
	if strings.Contains(got, `href="/tasks"`) {
		t.Error("Tasks is still on the strip; its signal belongs on the agents tile")
	}
}

// A task assigned to the agent is an agent working, and that is what the note
// on the agents tile reports. A task you kept for yourself is not.
func TestOnlyTheAgentsOwnWorkIsReportedAsWorking(t *testing.T) {
	acc := &auth.Account{ID: "worker", Name: "worker", Secret: "s"}
	auth.Create(acc)

	mine, err := tasks.Create(acc.ID, "Mine to do", "", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := tasks.Create(acc.ID, "For the agent", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(systemStrip(acc), "working") {
		t.Error("nothing is running yet, but the strip says something is working")
	}

	if _, err := tasks.Update(acc.ID, mine.ID, "", "", tasks.StatusDoing, "", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(systemStrip(acc), "working") {
		t.Error("a task you kept for yourself was reported as the agent working")
	}

	if _, err := tasks.Update(acc.ID, theirs.ID, "", "", tasks.StatusDoing, "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(systemStrip(acc), "1 working") {
		t.Errorf("the agent's own running task was not reported:\n%s", systemStrip(acc))
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

// The chips carry their question in a data attribute, never in an onclick.
//
// They used to be `onclick="…muChatAsk(" + JSString(s) + ")"`, and JSString
// returns a JSON string — double-quoted. Inside a double-quoted attribute the
// attribute ended at that quote, so the browser got the handler
// `window.muChatAsk&&window.muChatAsk(` and threw "Unexpected end of input".
// Every chip was dead, and the `&&` guard made a broken handler look
// indistinguishable from a missing one.
func TestChipsDoNotPutCodeInAnAttribute(t *testing.T) {
	for _, q := range []string{`What's happening?`, `Today's news`, `He said "hi"`, `a & b`} {
		markup := chipMarkup(q)

		if strings.Contains(markup, "onclick") {
			t.Errorf("a chip put code in an attribute: %s", markup)
		}
		// The attribute must survive the quotes in the question.
		if strings.Count(markup, `data-ask="`) != 1 {
			t.Errorf("the question broke its own attribute: %s", markup)
		}
		attr := markup[strings.Index(markup, `data-ask="`)+len(`data-ask="`):]
		attr = attr[:strings.Index(attr, `"`)]
		if html.UnescapeString(attr) != q {
			t.Errorf("the question did not survive escaping: %q -> %q", q, html.UnescapeString(attr))
		}
	}
}
