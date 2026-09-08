package agent

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/news"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHomeNewsShortcutReturnsFinalAnswerWithoutCardOrModelWork(t *testing.T) {
	if err := service.Register(news.Spec); err != nil {
		t.Fatal(err)
	}
	const who = "home_command"
	if err := auth.Create(&auth.Account{ID: who, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	old := CardContextFunc
	t.Cleanup(func() { CardContextFunc = old })
	CardContextFunc = func(string) string {
		t.Fatal("shortcut assembled ambient cards instead of returning directly")
		return ""
	}
	for _, prompt := range []string{"News", "news", " NEWS "} {
		body, _ := json.Marshal(map[string]any{"prompt": prompt, "cards": true})
		req := httptest.NewRequest("POST", "/agent", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		rec := httptest.NewRecorder()
		started := time.Now()
		handleQuery(rec, req)
		if time.Since(started) > 2*time.Second {
			t.Fatal("local shortcut took too long")
		}
		text := rec.Body.String()
		if !strings.Contains(text, `"type":"response"`) || !strings.Contains(text, `"type":"done"`) || strings.Contains(text, `"type":"error"`) {
			t.Fatalf("missing completion: %s", text)
		}
		if strings.Contains(text, "stream_token") || strings.Contains(text, "stream_start") {
			t.Fatal("web still emits partial prose")
		}
	}
	if _, ok := promptCommand("news", QueryOpts{CardContext: "ambient cards"}); !ok {
		t.Fatal("ambient context blocks shortcut")
	}
	if _, ok := promptCommand("news", QueryOpts{Extra: "attached article"}); ok {
		t.Fatal("attached material bypassed")
	}
}

func TestRecoveryIsBoundToTheOwnedRun(t *testing.T) {
	const who = "run_recovery"
	th := waiting(t, who)
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	flow := &Flow{ID: "recover-one", AccountID: who, ThreadID: th, Status: "done", Answer: "Recovered", HTML: `<div class="card">Recovered</div>`}
	if err := saveFlow(flow); err != nil {
		t.Fatal(err)
	}
	// A newer turn must not replace the answer to the run this browser started.
	saveFlow(&Flow{ID: "recover-other", AccountID: who, ThreadID: th, Status: "done", Answer: "Other", HTML: "Other"})
	for _, tc := range []struct {
		run, thread string
		want        bool
	}{{flow.ID, th, true}, {"missing", th, false}, {flow.ID, "wrong-thread", false}} {
		req := httptest.NewRequest("GET", "/agent/pending?thread="+tc.thread+"&flow="+tc.run, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		rec := httptest.NewRecorder()
		PendingHandler(rec, req)
		if strings.Contains(rec.Body.String(), "Recovered") != tc.want {
			t.Fatalf("wrong run returned: %s", rec.Body.String())
		}
	}
	auth.Create(&auth.Account{ID: "recovery_other"})
	other, _ := auth.CreateSession("recovery_other")
	req := httptest.NewRequest("GET", "/agent/pending?thread="+th+"&flow="+flow.ID, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: other.Token})
	rec := httptest.NewRecorder()
	PendingHandler(rec, req)
	if strings.Contains(rec.Body.String(), "Recovered") {
		t.Fatal("cross-account disclosure")
	}
}
