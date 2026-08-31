package agent

// The answer that landed while you were not looking.
//
// Reported as: asked the agent to build something, watched it say "working",
// refreshed, and never got an update again. Nothing was lost — the run
// finishes and writes its reply into the record whether or not anybody is
// holding the socket — but the reloaded page renders the conversation as it
// stands at that moment, which is the question and nothing after it, and then
// has no reason to look again.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

// waiting is a conversation whose last word is the person's, which is every
// conversation with a run in flight.
func waiting(t *testing.T, who string) string {
	t.Helper()
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck

	th := thread.Open(who, thread.WebClient, "pending-"+who)
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who,
		Role: thread.RolePerson, Text: "try build the app now"})
	return th.ID
}

// A question with no answer under it is a conversation still being answered.
func TestAQuestionWithNoAnswerIsPending(t *testing.T) {
	const who = "pendq"
	id := waiting(t, who)

	if !Pending(who, id) {
		t.Error("a question nobody has answered yet does not read as pending, " +
			"so the page renders it with nothing under it and never looks again")
	}

	// And once it is answered it stops being pending, or every reopened
	// conversation on the instance would spin forever.
	thread.Add(thread.Message{Thread: id, Account: who,
		Role: thread.RoleAgent, Text: "Created Live Sports at /apps/live-sports."})
	if Pending(who, id) {
		t.Error("an answered conversation still reads as pending")
	}

	// Nothing to be pending about.
	if Pending(who, "") || Pending("", id) {
		t.Error("a missing account or conversation reads as pending")
	}
}

// The endpoint says "not yet", then hands over the answer.
//
// This is the whole loop the browser runs: it asks while the run is in flight
// and is told to wait, and asks after it finishes and is given the turn to
// append. Both halves matter — a poll that could not say "still working" would
// have no way to tell a slow run from a dead one.
func TestPendingHandsOverTheAnswerWhenItLands(t *testing.T) {
	const who = "pendpoll"
	id := waiting(t, who)

	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	poll := func() (bool, string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/agent/pending?thread="+id, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		PendingHandler(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("the poll returned %d", w.Code)
		}
		var out struct {
			Waiting bool   `json:"waiting"`
			HTML    string `json:"html"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("the poll is not JSON: %v — %s", err, w.Body.String())
		}
		return out.Waiting, out.HTML
	}

	if wait, html := poll(); !wait || html != "" {
		t.Errorf("mid-run the poll says waiting=%v html=%q; it should say wait "+
			"and hand over nothing", wait, html)
	}

	// The run finishes. It writes into the record, which is what makes this
	// work at all — the browser that asked is long gone.
	thread.Add(thread.Message{Thread: id, Account: who,
		Role: thread.RoleAgent, Text: "Created **Live Sports** at /apps/live-sports."})

	wait, html := poll()
	if wait {
		t.Error("the answer has landed and the poll still says to wait")
	}
	if !strings.Contains(html, "Live Sports") {
		t.Errorf("the poll does not hand over the answer:\n%s", html)
	}
	// As the transcript draws it, not as a bare string. Two renderers for one
	// turn is how a reloaded conversation ends up styled differently below the
	// fold — see renderTurn, which both paths use.
	if !strings.Contains(html, `class="mu-agent"`) {
		t.Errorf("the answer is not rendered as a turn:\n%s", html)
	}
	// Markdown, rendered. The turn appended by the poll goes into the same
	// transcript as the ones rendered with the page.
	if strings.Contains(html, "**Live Sports**") {
		t.Errorf("the answer is handed over as raw markdown:\n%s", html)
	}
}

// Somebody else's conversation is not yours to poll.
func TestPendingWillNotReadSomebodyElsesConversation(t *testing.T) {
	const mine, other = "pendmine", "pendother"
	id := waiting(t, mine)
	auth.Create(&auth.Account{ID: other, Name: other}) //nolint:errcheck
	thread.Add(thread.Message{Thread: id, Account: mine,
		Role: thread.RoleAgent, Text: "a private answer"})

	sess, err := auth.CreateSession(other)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/agent/pending?thread="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	PendingHandler(w, r)

	if strings.Contains(w.Body.String(), "a private answer") {
		t.Errorf("the poll handed one account's conversation to another:\n%s",
			w.Body.String())
	}

	// And a stranger gets nothing at all.
	r = httptest.NewRequest(http.MethodGet, "/agent/pending?thread="+id, nil)
	w = httptest.NewRecorder()
	PendingHandler(w, r)
	if w.Code == http.StatusOK {
		t.Errorf("a signed-out caller was served the poll: %d %s", w.Code, w.Body.String())
	}
}
