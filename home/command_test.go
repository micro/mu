package home

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/thread"
	"mu/service/news"
)

func commandRequest(t *testing.T, owner, text, conversation string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"command": text, "thread": conversation})
	r := httptest.NewRequest("POST", "/command", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	if owner != "" {
		sess, err := auth.CreateSession(owner)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { auth.EndSession(sess.Token) })
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
	}
	w := httptest.NewRecorder()
	CommandHandler(w, r)
	return w
}

func TestCommandsUseDataAndPreserveConversation(t *testing.T) {
	const who = "command_reader"
	if err := auth.Create(&auth.Account{ID: who, Approved: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(who) })
	if err := service.Register(news.Spec); err != nil {
		t.Fatal(err)
	}
	w := commandRequest(t, who, "news", "", true)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"assistant":true`) {
		t.Fatalf("news did not run directly: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Thread string `json:"thread"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Thread == "" || len(thread.Messages(who, result.Thread, 10)) != 2 {
		t.Fatal("service result is missing from assistant context")
	}
	w = commandRequest(t, who, "Compare those headlines with yesterday", result.Thread, true)
	if !strings.Contains(w.Body.String(), `"assistant":true`) {
		t.Fatal("complex question did not reach the assistant")
	}
	foreign := thread.Open("somebody_else", thread.WebClient, "private")
	if got := commandRequest(t, who, "news", foreign.ID, true).Code; got != 404 {
		t.Fatalf("foreign thread accepted: %d", got)
	}
}

func TestAdminCommandsNeverReachModelOrTranscript(t *testing.T) {
	for _, a := range []*auth.Account{{ID: "command_admin", Admin: true, Approved: true}, {ID: "command_member", Approved: true}} {
		if err := auth.Create(a); err != nil {
			t.Fatal(err)
		}
		defer auth.DeleteAccount(a.ID)
	}
	t.Setenv("OPENAI_API_KEY", "")
	old := settings.Get("OPENAI_API_KEY")
	defer settings.Set("OPENAI_API_KEY", old)
	for _, who := range []string{"", "command_member"} {
		w := commandRequest(t, who, "admin config set OPENAI_API_KEY secret-test-value", "", true)
		if w.Code == 200 || strings.Contains(w.Body.String(), `"assistant":true`) {
			t.Fatal("non-admin reached configuration")
		}
	}
	if w := commandRequest(t, "command_admin", "admin config set OPENAI_API_KEY secret-test-value", "", false); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	for _, input := range []string{"admin config set OPENAI_API_KEY secret-test-value", "admin config list", "admin typo", "/admin config set OPENAI_API_KEY secret-test-value"} {
		w := commandRequest(t, "command_admin", input, "", true)
		if strings.Contains(w.Body.String(), "secret-test-value") || strings.Contains(w.Body.String(), `"assistant":true`) {
			t.Fatal("admin command leaked a credential or reached model")
		}
	}
	if settings.Get("OPENAI_API_KEY") != "secret-test-value" {
		t.Fatal("configuration was not changed")
	}
	if len(thread.List("command_admin", 10)) != 0 {
		t.Fatal("admin command entered model history")
	}
}

func TestCommandRendererEscapesUntrustedData(t *testing.T) {
	got := formatCommand(map[string]any{"items": []any{map[string]any{"title": "<script>alert(1)</script>", "url": "javascript:alert(1)"}}})
	if strings.Contains(got, "<script>") || strings.Contains(got, `href="javascript:`) {
		t.Fatal("tool data became executable HTML")
	}
}
