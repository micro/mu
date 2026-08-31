package inbox

// One inbox for everything.
//
// A note lived on /notes and a task on /tasks, so somebody with something on
// their mind had to decide which of three pages it belonged on before writing
// it. That is a filing decision demanded about a sentence that does not exist
// yet, and it is why people keep everything in their mail instead.
//
// Two properties, and the second is the one that decays. All three kinds are on
// the list; and none of them is copied into a store of the inbox's own, so the
// note this page shows is the note /notes edits.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/notes"
	"mu/service/tasks"
)

// inboxOf renders the list for one account, as the page does.
func inboxOf(t *testing.T, who, query string) string {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/inbox"+query, nil)
	w := httptest.NewRecorder()
	list(w, r, who, "")
	return w.Body.String()
}

// Notes and tasks are on the list, beside what arrived.
func TestTheInboxListsNotesAndTasksToo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "kindsreader"
	if err := auth.Create(&auth.Account{ID: who, Name: who}); err != nil {
		t.Fatal(err)
	}
	said(t, who, "mail", "<a@example.com>", "", "about the invoice")
	notes.Add(who, "Boiler service", "The engineer comes on the 14th")
	if _, err := tasks.Create(who, "Renew the passport", "Photos first", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}

	body := inboxOf(t, who, "")
	for _, want := range []string{"Boiler service", "Renew the passport"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q is not on the inbox — it is still only conversations:\n%s", want, body)
		}
	}
	// And each says which it is, because a column of mixed things that does not
	// is a column you have to open to identify.
	for _, want := range []string{">Note<", ">Task<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not label %s", want)
		}
	}
}

// And the kind filter narrows to one of them.
func TestTheKindFilterNarrowsTheList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "kindsfilter"
	if err := auth.Create(&auth.Account{ID: who, Name: who}); err != nil {
		t.Fatal(err)
	}
	said(t, who, "mail", "<b@example.com>", "", "about the roof")
	notes.Add(who, "Boiler service", "The engineer comes on the 14th")

	only := inboxOf(t, who, "?kind=note")
	if !strings.Contains(only, "Boiler service") {
		t.Errorf("the note is not on its own filter:\n%s", only)
	}
	if strings.Contains(only, "about the roof") {
		t.Errorf("filtering to notes still shows conversations:\n%s", only)
	}
}

// Nothing is copied: the inbox reads the stores the other pages write.
//
// This is the property that decays. A fourth store holding a shadow of the
// other three would pass every test above on the day it was written and drift
// the first time somebody edited a note on /notes — and there would be no answer
// to which copy is true. So: change it at the source, and the inbox says the new
// thing.
func TestTheInboxReadsTheNoteStoreRatherThanACopy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "kindssource"
	if err := auth.Create(&auth.Account{ID: who, Name: who}); err != nil {
		t.Fatal(err)
	}
	notes.Add(who, "Boiler service", "The engineer comes on the 14th")
	if body := inboxOf(t, who, ""); !strings.Contains(body, "comes on the 14th") {
		t.Fatalf("the note is not on the inbox at all:\n%s", body)
	}

	// Edited at the source, the way /notes does it.
	notes.Add(who, "Boiler service", "Moved to the 21st")
	body := inboxOf(t, who, "")
	if !strings.Contains(body, "Moved to the 21st") {
		t.Errorf("the inbox still shows the old text, so it is reading a copy:\n%s", body)
	}
	if strings.Contains(body, "comes on the 14th") {
		t.Errorf("the inbox shows both versions, so it kept one of its own:\n%s", body)
	}
}

// /inbox/new writes all three, through the services that own them.
func TestComposeWritesNotesAndTasks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "kindswriter"
	if err := auth.Create(&auth.Account{ID: who, Name: who}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}

	write := func(kind, title, body string) {
		t.Helper()
		form := url.Values{
			"kind": {kind}, "subject": {title}, "body": {body},
			"_csrf": {auth.CSRFToken(&http.Request{Header: http.Header{
				"Cookie": {"session=" + sess.Token}}})},
		}
		r := httptest.NewRequest(http.MethodPost, "/inbox/new", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		NewHandler(w, r)
		if w.Code != http.StatusSeeOther {
			t.Fatalf("writing a %s answered %d, want a redirect:\n%s", kind, w.Code, w.Body.String())
		}
	}

	write(kindNote, "Boiler service", "The engineer comes on the 14th")
	write(kindTask, "Renew the passport", "Photos first")

	// In the stores the other pages read, not in one of this page's own.
	found := false
	for _, n := range notes.All(who) {
		if n.Title == "Boiler service" && strings.Contains(n.Text, "14th") {
			found = true
		}
	}
	if !found {
		t.Error("the note was not written to internal/notes, which is what /notes reads")
	}
	found = false
	for _, task := range tasks.List(who, "") {
		if task.Title == "Renew the passport" {
			found = true
			if task.Assignee != tasks.Me {
				t.Errorf("a task written here was assigned to %q, want the person who wrote it", task.Assignee)
			}
		}
	}
	if !found {
		t.Error("the task was not written to service/tasks, which is what /tasks reads")
	}
}

// A reply is a message, whatever the picker last said.
//
// Offering to turn a reply into a note would be a control that throws away the
// conversation you were answering.
func TestAReplyIsAlwaysAMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "kindsreply"
	if err := auth.Create(&auth.Account{ID: who, Name: who}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	th := said(t, who, "mail", "<c@example.com>", "", "about the drains")

	r := httptest.NewRequest(http.MethodGet,
		"/inbox/new?kind=note&on="+url.QueryEscape(th.ID), nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	NewHandler(w, r)

	body := w.Body.String()
	if strings.Contains(body, "ib-new-kinds") {
		t.Errorf("a reply offers the kind picker:\n%s", body)
	}
	if !strings.Contains(body, `name="kind" value="`+kindMessage+`"`) {
		t.Errorf("a reply with ?kind=note is not forced back to a message:\n%s", body)
	}
}
