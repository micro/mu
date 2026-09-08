package home

import (
	"mu/internal/auth"
	"mu/internal/user"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStatusInlineEditor(t *testing.T) {
	if statusForm(httptest.NewRequest("GET", "/home", nil), "") != "" {
		t.Fatal("guest status editor")
	}
	markup := statusForm(httptest.NewRequest("GET", "/home", nil), "status-test")
	for _, unwanted := range []string{"<details", "<form", ">Save<", ">Clear<", ">Edit<"} {
		if strings.Contains(markup, unwanted) {
			t.Fatalf("unexpected control %s", unwanted)
		}
	}
	if !strings.Contains(markup, "data-status-input hidden") {
		t.Fatal("editor must start hidden")
	}
}

func TestStatusEditorLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	if out, err := exec.Command(node, "testdata/status.cjs").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestStatusSaveRequiresSession(t *testing.T) {
	r := httptest.NewRequest("POST", "/home", strings.NewReader("action=status&status=Hello"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	statusHandler(w, r)
	if w.Code != 401 {
		t.Fatalf("got %d, want unauthorized", w.Code)
	}
}

func TestStatusSaveOwnsIdentityAndRequiresCSRF(t *testing.T) {
	const who = "inline_status_owner"
	if err := auth.Create(&auth.Account{ID: who, Created: time.Now().Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	for _, valid := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/home", strings.NewReader(url.Values{"action": {"status"}, "status": {" Working   on Mu "}, "account_id": {"someone-else"}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Accept", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if valid {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		statusHandler(w, r)
		if !valid {
			if w.Code != 403 || user.Status(who) != "" {
				t.Fatalf("missing CSRF: %d", w.Code)
			}
		} else if w.Code != 200 || user.Status(who) != "Working on Mu" || user.Status("someone-else") != "" || !strings.Contains(w.Body.String(), `"status":"Working on Mu"`) {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
	}
}
