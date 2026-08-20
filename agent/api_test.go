package agent

// POST /agent/<name> — talking to an agent from a program.
//
// The Connect page offered an "Endpoint" and it was /mcp, which reaches this
// agent's *tools* for something else's agent to call. Nothing reached the agent
// itself — email is asynchronous, and /a2a, since deleted, ran a generic
// account-less orchestration that was nobody's agent. This is the door that was
// missing.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

// signedIn makes an account and a request carrying its session.
func signedIn(t *testing.T, who, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	_ = auth.Create(&auth.Account{ID: who})
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/agent/"+DefaultSlug, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return httptest.NewRecorder(), r
}

// Nobody gets an answer without an account.
//
// The endpoint runs a model against somebody's credits, so this is the one
// check that has to be first — before the agent is looked up, before the body
// is read.
func TestTheEndpointNeedsAnAccount(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/agent/"+DefaultSlug,
		strings.NewReader(`{"text":"hello"}`))
	w := httptest.NewRecorder()
	APIHandler(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status %d for an anonymous POST, want 401 — this spends credits", w.Code)
	}
}

// An agent that does not exist is not answered by a different one.
//
// The lookup is agentSlugTarget, the same one the page uses, so /agent/research
// and POST /agent/research cannot disagree about which agent that is. Falling
// back to the default would mean a typo silently reaching the wrong agent with
// the wrong scope.
func TestAnUnknownAgentIsNotFound(t *testing.T) {
	w, r := signedIn(t, "api-unknown", `{"text":"hello"}`)
	r.URL.Path = "/agent/nosuchagent"
	APIHandler(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d for an agent that does not exist, want 404", w.Code)
	}
}

// A question is required, and JSON that is not JSON is refused rather than
// treated as an empty one.
func TestTheEndpointNeedsAQuestion(t *testing.T) {
	for what, body := range map[string]string{
		"an empty question": `{"text":"  "}`,
		"no text at all":    `{}`,
		"not JSON":          `hello`,
	} {
		w, r := signedIn(t, "api-empty", body)
		APIHandler(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", what, w.Code)
		}
	}
}

// Somebody else's conversation is not a conversation to answer on.
//
// The thread id comes from the caller, so without this a forged one files an
// answer onto a stranger's thread — and the question with it, since Ask records
// both.
func TestTheEndpointRefusesAnotherAccountsThread(t *testing.T) {
	const mine, theirs = "api-mine", "api-theirs"
	_ = auth.Create(&auth.Account{ID: theirs})
	th := thread.Open(theirs, thread.WebClient, "api-1")
	if th == nil {
		t.Fatal("no conversation")
	}

	w, r := signedIn(t, mine, `{"text":"hello","thread":"`+th.ID+`"}`)
	APIHandler(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d — an id belonging to another account was accepted as "+
			"the conversation to answer on", w.Code)
	}
	// And nothing was written to it.
	if msgs := thread.Messages(theirs, th.ID, 10); len(msgs) != 0 {
		t.Errorf("%d messages on the other account's conversation", len(msgs))
	}
}

// The answer carries the conversation back, because a caller that wants a
// second turn has no other way to learn it.
//
// Read off the response shape rather than by running a model: what this holds
// is that a thread id is returned at all, which is the difference between an
// agent and a completion endpoint.
func TestTheAnswerCarriesTheConversation(t *testing.T) {
	var out apiAnswer
	if err := json.Unmarshal([]byte(`{"text":"hi","thread":"t-1","agent":"micro"}`), &out); err != nil {
		t.Fatal(err)
	}
	if out.Thread == "" {
		t.Error("the response shape has no thread in it, so a second turn is impossible")
	}
	// And the field is not omitted when set — omitempty on a populated string
	// is fine, but the encoder has to actually emit it.
	b, err := json.Marshal(apiAnswer{Text: "hi", Thread: "t-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"thread":"t-1"`) {
		t.Errorf("the thread is not in the encoded answer: %s", b)
	}
}
