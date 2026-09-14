package inbox

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/thread"
)

// said starts a conversation with an agent and puts a line in it.
func said(t *testing.T, owner, client, key, agentID, text string) *thread.Thread {
	t.Helper()
	th := thread.Open(owner, client, key)
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	if agentID != "" {
		thread.SetAgent(owner, th.ID, agentID)
	}
	thread.Add(thread.Message{Thread: th.ID, Account: owner, Text: text})
	return th
}

// The inbox lists conversations, whichever client each arrived on.
//
// That is what makes it one inbox rather than five: an email chain, a WhatsApp
// exchange and a chat on this page are the same kind of thing in the record.
func TestTheInboxListsEveryConversation(t *testing.T) {
	const who = "inbox_lister"
	said(t, who, "mail", "<a@example.com>", "", "about the invoice")
	said(t, who, "whatsapp", "44700900000", "", "are you around")

	body := listBody(t, "/inbox", who, "")
	for _, want := range []string{"about the invoice", "are you around"} {
		if !strings.Contains(body, want) {
			t.Errorf("the inbox is missing %q", want)
		}
	}
	// Where it happened, when that is not here.
	if !strings.Contains(body, "Mail") || !strings.Contains(body, "WhatsApp") {
		t.Errorf("the rows do not say where they happened:\n%s", body)
	}
}

// An agent is a mailbox. What arrives for the research agent is its mail, not a
// slice of yours, so it gets a box of its own with a way in and out.

// A switcher with one destination is a control that cannot do anything.
func TestNoSwitcherWhenNothingHasAnAgent(t *testing.T) {
	const who = "inbox_one_box"
	said(t, who, thread.WebClient, "only", "", "hello")

	// The markup, not the stylesheet — mu.css always carries the rule.
	if body := listBody(t, "/inbox", who, ""); strings.Contains(body, `<div class="ib-boxes">`) {
		t.Error("an account with no agent conversations is offered a switcher")
	}
}

// An empty box says which box is empty. The narrower fact is the true one, and
// the address is already on the page above it.

func listBody(t *testing.T, path, accountID, box string) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", path, nil)
	q := r.URL.Query()
	q.Set("view", "history")
	r.URL.RawQuery = q.Encode()
	priority(w, r, accountID)
	return w.Body.String()
}

// searchBody is a search the way the page does one: in the body of a POST.
//
// It was listBody with the term in the path, which is how the search worked
// before it stopped putting what somebody typed into the URL. A test that goes
// on asking with ?q= would pass against a handler that reads the query, which is
// exactly the handler we are trying not to have. See AGENTS.md, "What may travel
// in a URL".
func searchBody(t *testing.T, term, accountID, box string) string {
	t.Helper()
	form := url.Values{"q": {term}}
	r := httptest.NewRequest(http.MethodPost, "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	priority(w, r, accountID)
	return w.Body.String()
}
