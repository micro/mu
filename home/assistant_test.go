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
			ns := "landing"
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
			Index(w, r)
			body := w.Body.String()
			if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("%s: %d %v", path, w.Code, w.Header())
			}
			if !strings.Contains(body, `"storageNS":"`+ns+`"`) {
				t.Errorf("%s: missing isolated namespace %s", path, ns)
			}
			if who != "" && (strings.Contains(body, `class="chat-sess-list"`) || !strings.Contains(body, `mu-chat-transcript`)) {
				t.Error("Conversation must show its transcript without a history switcher")
			}
			if !strings.Contains(body, `id="mu-chat-location"`) {
				t.Errorf("%s: location control does not match authentication", path)
			}
			if strings.Contains(body, `id="home-cards"`) {
				t.Error("Assistant depends on Home")
			}
		}
	}
}
