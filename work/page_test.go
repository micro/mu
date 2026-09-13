package work

import (
	"mu/internal/auth"
	"mu/service/tasks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkPageRequiresOwnerAndShowsOnlyDelegatedJobs(t *testing.T) {
	for _, who := range []string{"work_page_owner", "work_page_other"} {
		if err := auth.Create(&auth.Account{ID: who, Secret: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	job, err := tasks.Create("work_page_owner", "Delegated goal", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create("work_page_owner", "Personal chore", "", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		who, path string
		status    int
	}{{"", "/work", 303}, {"work_page_other", "/work?id=" + job.ID, 404}, {"work_page_owner", "/work", 200}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.who != "" {
			sess, err := auth.CreateSession(tc.who)
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		if tc.who == "" {
			if w.Code < 300 || w.Code >= 400 {
				t.Fatalf("anonymous %d", w.Code)
			}
			continue
		}
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.who, tc.path, w.Code)
		}
		if tc.who == "work_page_owner" {
			if !strings.Contains(w.Body.String(), "Delegated goal") || strings.Contains(w.Body.String(), "Personal chore") {
				t.Fatal("work page mixes delegated and personal tasks")
			}
		}
	}
}
