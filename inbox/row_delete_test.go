package inbox

// Throwing something away without reading it.
//
// Delete was on the conversation and nowhere else, so discarding a thread you
// can already see is junk meant opening it — which marks it read on the way in.
// Reading something in order to discard it is the one interaction a mailbox
// exists to save you, which is why every mail client puts delete on the row.

import (
	"strings"
	"testing"
)

// Every row offers it, and offers it for its own conversation.
func TestEveryRowCanBeDeleted(t *testing.T) {
	const who = "inbox_rowdel"
	first := said(t, who, "mail", "<a@example.com>", "", "the first one")
	second := said(t, who, "mail", "<b@example.com>", "", "the second one")

	body := listBody(t, "/inbox", who, "")
	if n := strings.Count(body, `action="/inbox/delete"`); n != 2 {
		t.Errorf("%d delete forms for 2 conversations — every row gets one", n)
	}
	// Each carries the id of the row it sits on, or one of them deletes the
	// other's conversation.
	for _, th := range []string{first.ID, second.ID} {
		if !strings.Contains(body, `name="id" value="`+th+`"`) {
			t.Errorf("no delete form carries the id %s", th)
		}
	}
}

// It posts, and it carries the token, or the handler refuses it — StrictCSRF is
// what /inbox/delete checks, and a form without one is a button that does
// nothing.
func TestTheRowDeleteIsAPostWithAToken(t *testing.T) {
	const who = "inbox_rowdel_csrf"
	said(t, who, "mail", "<a@example.com>", "", "something")

	body := listBody(t, "/inbox", who, "")
	if !strings.Contains(body, `method="post"`) {
		t.Error("the delete is not a POST, so it is a link that changes state")
	}
	if !strings.Contains(body, `name="_csrf"`) {
		t.Error("no CSRF token, so /inbox/delete will refuse it")
	}
	if !strings.Contains(body, "confirm(") {
		t.Error("nothing asks first, and thread.Delete does not come back")
	}
}

// The form is beside the link, never inside it. A <form> nested in an <a> is
// invalid and a submit inside a navigation target means one click has two
// meanings — the row would open the conversation it was asked to delete.
func TestTheDeleteIsNotInsideTheRowLink(t *testing.T) {
	const who = "inbox_rowdel_nest"
	said(t, who, "mail", "<a@example.com>", "", "something")

	body := listBody(t, "/inbox", who, "")
	i := strings.Index(body, `<a class="ib-row`)
	if i < 0 {
		t.Fatal("no row on the page")
	}
	end := strings.Index(body[i:], "</a>")
	if end < 0 {
		t.Fatal("the row link is never closed")
	}
	if strings.Contains(body[i:i+end], "<form") {
		t.Error("the delete form is inside the row's <a>")
	}
}
