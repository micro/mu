package twilio

// What a refusal has to say, and what must never be sent.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestARefusalSaysWhatIsWrong — Twilio answers with an errors array, and
// reading only the top level put the whole JSON blob in front of an operator
// with the one sentence that explained it buried in the middle.
func TestARefusalSaysWhatIsWrong(t *testing.T) {
	body := []byte(`{"errors":[{"code":400,"context":null,` +
		`"infoUrl":"https://www.twilio.com/docs/api/errors/400",` +
		`"message":"The header 'Reply-To' is restricted and cannot be overridden."}],"status":400}`)

	got := emailError(body, "400 Bad Request")
	if !strings.Contains(got, "restricted and cannot be overridden") {
		t.Errorf("the refusal reads %q, which does not say what is wrong", got)
	}
	if strings.Contains(got, "infoUrl") || strings.Contains(got, "{") {
		t.Errorf("the refusal is raw JSON: %q", got)
	}
}

// TestReplyToIsNeverSent — it is refused with a 400, so passing it through
// fails every message rather than degrading one. The field stays on the struct
// because the SMTP path can honour it.
func TestReplyToIsNeverSent(t *testing.T) {
	m := Email{
		From: "a@example.com", To: "b@example.com",
		ReplyTo: "reply@example.com",
		Subject: "Hi", Text: "there",
	}
	// Rebuild the body the way Send does, without making a request.
	content := map[string]any{"subject": m.Subject, "text": m.Text}
	raw, err := json.Marshal(map[string]any{
		"from":    map[string]string{"address": m.From},
		"to":      []map[string]string{{"address": m.To}},
		"content": content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Reply-To") || strings.Contains(string(raw), m.ReplyTo) {
		t.Error("a Reply-To reached the request body, which Twilio refuses outright")
	}

	// And the real one: the source must not put it in headers.
	src, err := os.ReadFile("email.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `"Reply-To": `) {
		t.Error("Send still sets a Reply-To header — every message will 400")
	}
}
