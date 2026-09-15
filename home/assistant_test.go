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
			if who != "" {
				auth.Create(&auth.Account{ID: who, Name: who})
				sess, err := auth.CreateSession(who)
				if err != nil {
					t.Fatal(err)
				}
				r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
			}
			r.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()
			Index(w, r)
			body := w.Body.String()
			if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("%s: %d %v", path, w.Code, w.Header())
			}
			if who == "" && !strings.Contains(body, `"account":null`) {
				t.Fatal("guest received account data")
			}
			if who != "" && !strings.Contains(body, `"id":"`+who+`"`) {
				t.Fatal("wrong account data")
			}
			if strings.Contains(body, `"secret"`) || strings.Contains(body, `"token"`) {
				t.Fatal("credential exposed")
			}
		}
	}
}
