package inbox

import (
	"mu/internal/thread"
	"mu/service/mail"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComposeChoiceAndReply(t *testing.T) {
	for _, tc := range []struct {
		mode, to string
		want     string
	}{
		{"assistant", "", `value="assistant" checked`},
		{"email", "friend@example.com", `value="email" checked`},
	} {
		r := httptest.NewRequest("GET", "/inbox/new", nil)
		w := httptest.NewRecorder()
		writeOne(w, r, "compose_test", form{Mode: tc.mode, To: tc.to, Kind: kindMessage})
		body := w.Body.String()
		if !strings.Contains(body, tc.want) {
			t.Fatalf("choice missing: %s", tc.mode)
		}
		if tc.mode == "assistant" && !strings.Contains(body, `required hidden disabled`) {
			t.Fatal("assistant asks for a recipient")
		}
		if !strings.Contains(body, `name="body"`) || !strings.Contains(body, `name="subject"`) {
			t.Fatal("composer fields missing")
		}
	}
	for _, tc := range []struct {
		to  string
		ask bool
	}{
		{mail.AgentMailbox, false},
		{mail.AgentMailbox + "@" + mail.ConfiguredDomain(), false},
		{"agent@unrelated.example", true},
		{"friend@example.com", true},
	} {
		bar := actionBar(&thread.Thread{Client: "mail"}, tc.to, true)
		if strings.Contains(bar, "Ask Micro") != tc.ask {
			t.Errorf("wrong action for %s", tc.to)
		}
		if !strings.Contains(bar, ">Reply</a>") {
			t.Errorf("reply missing for %s", tc.to)
		}
	}
}
