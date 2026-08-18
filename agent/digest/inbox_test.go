package digest

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
)

func reader(t *testing.T, id string) {
	t.Helper()
	if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
		t.Fatalf("could not create %s: %v", id, err)
	}
}

// The briefing turns up without being asked for.
//
// It was published as a blog post and nothing else, so the one thing the agent
// does on its own every day was visible only to somebody who went looking. An
// inbox that only ever holds conversations you started is a chat history; this
// is the first thing that arrives in it on its own, which is the claim the
// product makes.
func TestTheBriefingArrivesInTheInbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DIGEST_INBOX", "")

	const who = "brief-reader"
	reader(t, who)

	deliver("Daily Digest — 18 Aug 2026", "Markets were quiet and the news was not.")

	threads := thread.List(who, 10)
	if len(threads) == 0 {
		t.Fatal("nothing arrived")
	}
	msgs := thread.Messages(who, threads[0].ID, 10)
	if len(msgs) != 1 {
		t.Fatalf("the conversation holds %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].Text, "Markets were quiet") {
		t.Errorf("the brief is not in it: %q", msgs[0].Text)
	}
	// From the agent, because nobody typed it.
	if msgs[0].Role != thread.RoleAgent {
		t.Errorf("the brief is recorded as %q", msgs[0].Role)
	}
	// And it says where it came from, which is neither the web nor mail.
	if threads[0].Client != Client {
		t.Errorf("the conversation says it happened on %q", threads[0].Client)
	}
}

// Running twice in a day does not deliver twice. A retry after a failure is
// what this protects: the day is the key, so the second run finds the
// conversation rather than making another one.
func TestASecondRunOnTheSameDayDoesNotDeliverTwice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DIGEST_INBOX", "")

	const who = "brief-twice"
	reader(t, who)

	deliver("Daily Digest", "The first body.")
	deliver("Daily Digest", "A second attempt.")

	threads := thread.List(who, 10)
	if len(threads) != 1 {
		t.Fatalf("two runs made %d conversations, want 1", len(threads))
	}
	if msgs := thread.Messages(who, threads[0].ID, 10); len(msgs) != 1 {
		t.Fatalf("two runs left %d messages, want 1", len(msgs))
	}
}

// An operator running this for other people can decide a daily arrival is not
// what their account holders signed up for.
func TestAnOperatorCanTurnTheArrivalOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DIGEST_INBOX", "false")

	const who = "brief-off"
	reader(t, who)

	deliver("Daily Digest", "Should not arrive.")

	if got := thread.List(who, 10); len(got) != 0 {
		t.Errorf("the brief arrived anyway: %d conversations", len(got))
	}
}

// Nothing to say means nothing delivered. An empty brief is a generation that
// failed, and a conversation holding nothing is worse than no conversation.
func TestAnEmptyBriefIsNotDelivered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DIGEST_INBOX", "")

	const who = "brief-empty"
	reader(t, who)

	deliver("Daily Digest", "")

	if got := thread.List(who, 10); len(got) != 0 {
		t.Errorf("an empty brief was delivered: %d conversations", len(got))
	}
}
