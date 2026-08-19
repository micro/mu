package agent

// No raw tool payload ever reaches a person.
//
// The freshness guard replaces a suspect answer with a rendering of the tool
// results themselves, so whatever formatToolResult produced becomes the thing
// somebody reads. For any tool with no formatter that was the literal response
// envelope — which is how an instruction to turn a sender down politely came
// back as `{"text":"Your inbox (5 messages):\n- ..."}`.

import "testing"

func TestTheResponseEnvelopeIsNotAnAnswer(t *testing.T) {
	unwrapped := map[string]string{
		`{"text":"Your inbox (5 messages):\n- one\n"}`:  "Your inbox (5 messages):\n- one\n",
		`{"text":"Sent (1):\n- someone@example.com\n"}`: "Sent (1):\n- someone@example.com\n",
	}
	for in, want := range unwrapped {
		if got := plainToolText(in); got != want {
			t.Errorf("plainToolText(%q) = %q, want the text out of it (%q)", in, got, want)
		}
	}

	// Left alone. A formatter's job is to keep what is in a structured result,
	// so unwrapping is only safe where there is nothing else to lose.
	for what, in := range map[string]string{
		"more than one field": `{"text":"x","url":"y"}`,
		"an array":            `[{"text":"x"}]`,
		"not JSON at all":     `Your inbox (5 messages):`,
		"an empty text":       `{"text":"  "}`,
		"no text field":       `{"count":3}`,
	} {
		if got := plainToolText(in); got != in {
			t.Errorf("%s: plainToolText(%q) = %q, want it passed through untouched", what, in, got)
		}
	}
}

// And the default case is what does it, so a service written next week is
// covered without anybody remembering to add it to a list.
func TestAToolWithNoFormatterStillReadsAsText(t *testing.T) {
	const envelope = `{"text":"3 flood warnings in force"}`
	if got := formatToolResult("hazards_floods", envelope, nil); got != "3 flood warnings in force" {
		t.Errorf("a tool nobody wrote a formatter for rendered as %q — the switch in "+
			"formatToolResult names the tools somebody got to, and everything after "+
			"it has to read as prose without being added to that list", got)
	}
}
