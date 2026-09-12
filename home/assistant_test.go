package home

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAssistantSeparatesAccountAndGuestConversations(t *testing.T) {
	for _, who := range []string{"", "asstreadera", "asstreaderb"} {
		for _, path := range []string{"/assistant", "/assistant?view=home"} {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			ns := "assistant:guest"
			if who != "" {
				auth.Create(&auth.Account{ID: who, Name: who})
				sess, err := auth.CreateSession(who)
				if err != nil {
					t.Fatal(err)
				}
				r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
				ns = "agent-" + who + "-"
			}
			w := httptest.NewRecorder()
			AssistantHandler(w, r)
			body := w.Body.String()
			if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("%s: %d %v", path, w.Code, w.Header())
			}
			if !strings.Contains(body, `var NS="`+ns+`"`) {
				t.Errorf("%s: missing isolated namespace %s", path, ns)
			}
			if who != "" && (!strings.Contains(body, `class="chat-sess-list"`) || !strings.Contains(body, `mu-chat-transcript`)) {
				t.Error("Assistant must show saved conversations and a transcript")
			}
			if strings.Contains(body, `id="mu-chat-location"`) != (who != "") {
				t.Errorf("%s: location control does not match authentication", path)
			}
			if strings.Contains(body, `id="home-cards"`) {
				t.Error("Assistant depends on Home")
			}
		}
	}
}

func TestHomeDisclosureFollowsAnswer(t *testing.T) {
	body := homeFor(t, "homedisclose")
	form := strings.Index(body, `id="mu-chat-form"`)
	answer := strings.Index(body, `id="mu-chat-conv"`)
	toggle := strings.Index(body, `id="home-conversation-actions"`)
	if form < 0 || answer <= form || toggle <= answer {
		t.Fatal("Home input, answers, disclosure must appear in that order")
	}
	if !strings.Contains(body, `var CONTINUE_NS="assistant:account:homedisclose:home";`) {
		t.Error("Home handoff must target its separate conversation")
	}
	if !strings.Contains(body, "var stationary=true;") {
		t.Error("Home must preserve its surrounding content")
	}
}
