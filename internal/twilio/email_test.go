package twilio

// What a refusal has to say, and what must never be sent.

import (
	"encoding/json"
	"reflect"
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

// TestTheRequestIsTheDocumentedOne — the request built here, against the one in
// Twilio's own example, field for field.
//
// This is the check that matters: everything else about this service is policy,
// and the only thing that decides whether a message goes is whether the body is
// the shape Twilio accepts. It was not, once — a Reply-To in content.headers is
// refused outright and every message 400'd.
func TestTheRequestIsTheDocumentedOne(t *testing.T) {
	got := emailBody(Email{
		From: "support@example.com", FromName: "Support Team",
		To: "john.doe@example.com", ToName: "John Doe",
		Subject: "Your subject line",
		HTML:    "<p>Your message content in HTML format.</p>",
		Text:    "Your message content in plain text.",
		// Asked for, and deliberately not carried: Twilio refuses it.
		ReplyTo: "reply@example.com",
	})

	// Twilio's documented example, verbatim.
	want := map[string]any{
		"from": map[string]string{"address": "support@example.com", "name": "Support Team"},
		"to": []map[string]string{
			{"address": "john.doe@example.com", "name": "John Doe"},
		},
		"content": map[string]any{
			"subject": "Your subject line",
			"html":    "<p>Your message content in HTML format.</p>",
			"text":    "Your message content in plain text.",
		},
	}

	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	var ga, gb any
	json.Unmarshal(a, &ga) //nolint:errcheck
	json.Unmarshal(b, &gb) //nolint:errcheck
	if !reflect.DeepEqual(ga, gb) {
		t.Errorf("the request does not match Twilio's documented one\n got: %s\nwant: %s", a, b)
	}
	if strings.Contains(string(a), "Reply-To") || strings.Contains(string(a), "reply@example.com") {
		t.Error("a Reply-To reached the request body, which Twilio refuses outright")
	}
}

// TestOptionalFieldsAreOmittedRatherThanEmpty — a name nobody set must not go
// as "", and a message with no HTML part must not carry an empty one.
func TestOptionalFieldsAreOmittedRatherThanEmpty(t *testing.T) {
	got := emailBody(Email{From: "a@example.com", To: "b@example.com", Subject: "Hi", Text: "there"})
	raw, _ := json.Marshal(got)
	for _, absent := range []string{`"name"`, `"html"`} {
		if strings.Contains(string(raw), absent) {
			t.Errorf("%s is in the request and was never set: %s", absent, raw)
		}
	}
	if !strings.Contains(string(raw), `"text":"there"`) {
		t.Errorf("the body is missing its text part: %s", raw)
	}
}
