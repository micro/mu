package inbox

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

func itemCaller(t *testing.T, owner string) func(string, url.Values, bool) *httptest.ResponseRecorder {
	t.Helper()
	if err := auth.Create(&auth.Account{ID: owner, Name: owner}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	return func(path string, form url.Values, csrf bool) *httptest.ResponseRecorder {
		method := http.MethodGet
		if form != nil {
			method = http.MethodPost
		}
		probe := httptest.NewRequest(method, path, nil)
		probe.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			form.Set("_csrf", auth.CSRFToken(probe))
		}
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Handler(w, r)
		return w
	}
}

func TestInboxNoteReadEditDeleteAndOwnership(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	call := itemCaller(t, "inbox_note_owner")
	other := itemCaller(t, "inbox_note_other")
	notes.Add("inbox_note_owner", "Private title", "**Original note**")
	n := notes.All("inbox_note_owner")[0]
	path := "/inbox?kind=note&id=" + n.ID
	if n.ID == "" || strings.Contains(noteRow(n), `href="/notes`) {
		t.Fatal("note does not open inside inbox")
	}
	w := call(path, nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<strong>Original note</strong>") {
		t.Fatalf("read: %d %s", w.Code, w.Body.String())
	}
	if w = other(path, nil, false); w.Code != 404 {
		t.Fatalf("cross-account read: %d", w.Code)
	}
	if w = other(path, url.Values{"action": {"delete"}}, true); w.Code != 404 {
		t.Fatalf("cross-account delete: %d", w.Code)
	}
	if w = call(path, url.Values{"action": {"save"}, "text": {"Changed"}}, false); w.Code != 403 {
		t.Fatalf("missing CSRF: %d", w.Code)
	}
	if edit := call(path+"&edit=1", nil, false); edit.Code != 200 || !strings.Contains(edit.Body.String(), `name="text"`) {
		t.Fatal("note editor missing")
	}
	w = call(path, url.Values{"action": {"save"}, "text": {"Changed"}}, true)
	if w.Code != 303 || w.Header().Get("Location") != path || notes.Get("inbox_note_owner", n.Title) != "Changed" {
		t.Fatal("edit did not update original note and stay in inbox")
	}
	if notes.All("inbox_note_owner")[0].ID != n.ID {
		t.Fatal("editing changed note address")
	}
	w = call(path, url.Values{"action": {"delete"}}, true)
	if w.Header().Get("Location") != "/inbox" || notes.Get("inbox_note_owner", n.Title) != "" {
		t.Fatal("delete failed")
	}
	if w = call(path, nil, false); w.Code != 404 {
		t.Fatal("deleted note still readable")
	}
}

func TestInboxTaskControlsAndOwnership(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	call := itemCaller(t, "inbox_task_owner")
	other := itemCaller(t, "inbox_task_other")
	task, err := tasks.Create("inbox_task_owner", "Test work", "Details", tasks.Me, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/inbox?kind=task&id=" + task.ID
	w := call(path, nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Test work") || (strings.Contains(w.Body.String(), `action="/tasks/`) || strings.Contains(w.Body.String(), "&amp;action=")) {
		t.Fatal("task controls leave inbox")
	}
	if strings.Contains(taskRow(task), `href="/tasks`) {
		t.Fatal("task row leaves inbox")
	}
	if w = other(path, nil, false); w.Code != 404 {
		t.Fatal("other account can read task")
	}
	if w = other(path, url.Values{"action": {"done"}}, true); w.Code != 404 {
		t.Fatal("other account can change task")
	}
	if w = call(path, url.Values{"action": {"done"}}, false); w.Code != 403 {
		t.Fatal("task action needs CSRF")
	}
	for _, action := range []string{"done", "reopen"} {
		w = call(path, url.Values{"action": {action}}, true)
		if w.Code != 303 || w.Header().Get("Location") != path {
			t.Fatal("task action leaves inbox")
		}
		got, _ := tasks.Get("inbox_task_owner", task.ID)
		if (action == "done") != (got.Status == tasks.StatusDone) {
			t.Fatal("original task not updated")
		}
	}
	if _, err := tasks.Update("inbox_task_owner", task.ID, "", "", tasks.StatusDone, "", "**Finished work**"); err != nil {
		t.Fatal(err)
	}
	if result := call(path, nil, false); !strings.Contains(result.Body.String(), "<strong>Finished work</strong>") {
		t.Fatal("task result not rendered in inbox")
	}
	w = call(path, url.Values{"action": {"delete"}}, true)
	if w.Header().Get("Location") != "/inbox" {
		t.Fatal("delete leaves inbox")
	}
	if _, err := tasks.Get("inbox_task_owner", task.ID); err == nil {
		t.Fatal("task not deleted")
	}
}
