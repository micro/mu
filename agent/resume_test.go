package agent

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMicroEntryReopensHistoryAndNewStartsEmpty(t *testing.T) {
	const who = "micro_entry_test"
	if err := auth.Create(&auth.Account{ID: who}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	id := Opened(who, thread.WebClient, "entry-test", "", "")
	Said(who, id, "remember this conversation", "", "")
	Answered(who, id, "a remembered answer", "")
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/?session=" + id, true},
		{"/?new=1", false},
		{"/agent/micro", true},
		{"/agent/micro?session=" + id, true},
		{"/agent/micro?new=1", false},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		servePage(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if got := strings.Contains(w.Body.String(), `var contextId="`+id+`"`); got != tc.want {
			t.Errorf("%s reopened=%v", tc.path, got)
		}
		if !strings.Contains(w.Body.String(), `agent-micro_entry_test-`) {
			t.Error("storage lacks account scope")
		}
	}
}
