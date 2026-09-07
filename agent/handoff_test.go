package agent

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuestHandoffIsPrivateAndRetryable(t *testing.T) {
	const who = "handoff_owner"
	if err := auth.Create(&auth.Account{ID: who}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	body := `{"turns":[{"prompt":"keep this question","answer":"and this answer"}]}`
	request := func(token bool) *http.Request {
		r := httptest.NewRequest("POST", "/agent/handoff", strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if token {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	bad := httptest.NewRecorder()
	HandoffHandler(bad, request(false))
	if bad.Code != 403 {
		t.Fatalf("missing CSRF: %d", bad.Code)
	}
	id := ""
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		HandoffHandler(w, request(true))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var result map[string]string
		json.Unmarshal(w.Body.Bytes(), &result)
		if id != "" && id != result["id"] {
			t.Fatal("retry created a second thread")
		}
		id = result["id"]
	}
	if n := len(thread.Messages(who, id, 10)); n != 2 {
		t.Fatalf("saved %d messages", n)
	}
	if thread.Get("someone_else", id) != nil {
		t.Fatal("handoff crossed accounts")
	}
	r := httptest.NewRequest("GET", "/?session="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	MicroHandler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "keep this question") {
		t.Fatal("root did not reopen imported conversation")
	}
}
