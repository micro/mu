package inbox

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

// The inbox can be searched, over the record and not over the page.
//
// thread.Search was exported and complete and the only door onto it was
// /recall, in another service, on another page. So the screen holding every
// conversation this account has had could not be looked through — you had to
// know a different page searched the same store.
func TestTheInboxCanBeSearched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "searcher"

	said(t, who, mailClient, "henrik@example.com", "", "Invoice 4021: attached is October.")
	said(t, who, thread.SMSClient, "+447700900123", "", "running ten minutes late")

	// A hit, on mail.
	body := listBody(t, "/inbox?q=invoice", who, "")
	if !strings.Contains(body, "Invoice 4021") {
		t.Error("searching for a word in a mail conversation did not find it")
	}
	if strings.Contains(body, "running ten minutes late") {
		t.Error("the search returned a conversation that does not match")
	}

	// And on a text, from the same box, which is the point of it being one
	// inbox: the record does not care which protocol carried the sentence.
	body = listBody(t, "/inbox?q=ten+minutes", who, "")
	if !strings.Contains(body, "running ten minutes late") {
		t.Error("a text was not searchable from the inbox — the search is not " +
			"over the record, it is over the mail")
	}

	// The line that matched is the preview, not the last thing said. A results
	// list showing the tail of each conversation makes you open every one.
	if !strings.Contains(body, "running ten minutes late") {
		t.Error("the matching line is not shown")
	}

	// Nothing found says so, rather than falling back to the whole mailbox.
	body = listBody(t, "/inbox?q=zzzznothing", who, "")
	if strings.Contains(body, "Invoice 4021") {
		t.Error("a search that matched nothing showed the inbox instead, which " +
			"reads as though everything matched")
	}
	if !strings.Contains(body, "Nothing matches") {
		t.Error("a search that matched nothing said nothing about it")
	}
}

// The box is not lost when you search inside it.
func TestSearchingInsideAMailboxStaysInIt(t *testing.T) {
	if got := searchBox("research", "invoice"); !strings.Contains(got, `action="/inbox/research"`) {
		t.Errorf("the search form leaves the mailbox: %s", got)
	}
	if got := searchBox("", ""); !strings.Contains(got, `action="/inbox"`) {
		t.Errorf("the search form has no action: %s", got)
	}
	// And there is nothing to clear until something has been searched for.
	if strings.Contains(searchBox("", ""), "Clear") {
		t.Error("an empty search box offers to clear itself")
	}
	if !strings.Contains(searchBox("", "invoice"), "Clear") {
		t.Error("a search offers no way back to the mailbox")
	}
}
