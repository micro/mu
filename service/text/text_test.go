package text

import (
	"mu/internal/service"
	"strings"
	"testing"
)

// The cap is the whole answer to a fixed price over a variable cost, so it has
// to actually bind — and it has to say when it did, or a caller reads a partial
// answer as a complete one.
func TestInputIsCappedAndSaysSo(t *testing.T) {
	long := strings.Repeat("a", maxInput+5000)

	got, clipped := clip(long)
	if !clipped {
		t.Fatal("oversized input was not clipped")
	}
	if len(got) != maxInput {
		t.Errorf("clipped to %d characters, want %d", len(got), maxInput)
	}

	out := withNote("a summary", true, len(long))
	if !strings.Contains(out, "Only the first") {
		t.Errorf("a clipped answer does not say so: %q", out)
	}
}

// Text within the cap must come back untouched and unannotated — a note on an
// answer that was not truncated is a lie in the other direction.
func TestShortInputIsUntouched(t *testing.T) {
	got, clipped := clip("  hello  ")
	if clipped {
		t.Error("short input was reported as clipped")
	}
	if got != "hello" {
		t.Errorf("got %q, want the trimmed text", got)
	}
	if out := withNote("answer", false, 5); out != "answer" {
		t.Errorf("an untruncated answer gained a note: %q", out)
	}
}

// Models wrap JSON in a fence however firmly they are told not to. A caller
// parsing extract output wants JSON, not an apology about markdown.
func TestFencedJSONIsUnwrapped(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":1}\n```":     `{"a":1}`,
		`{"a":1}`:                 `{"a":1}`,
		"  {\"a\":1}  ":           `{"a":1}`,
	}
	for in, want := range cases {
		if got := unfence(in); got != want {
			t.Errorf("unfence(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every method needs its text, and the three that take a second argument need
// that too — refused here rather than spending a model call to find out.
func TestMissingArgumentsAreRefusedBeforeSpending(t *testing.T) {
	var s Server

	if err := s.Summarise(nil, &SummariseRequest{}, &SummariseResponse{}); err == nil {
		t.Error("summarise accepted empty text")
	}
	if err := s.Extract(nil, &ExtractRequest{Text: "x"}, &ExtractResponse{}); err == nil {
		t.Error("extract accepted a missing schema")
	}
	if err := s.Extract(nil, &ExtractRequest{Text: "x", Schema: strings.Repeat("s", 5000)}, &ExtractResponse{}); err == nil {
		t.Error("extract accepted an oversized schema")
	}
	if err := s.Classify(nil, &ClassifyRequest{Text: "x"}, &ClassifyResponse{}); err == nil {
		t.Error("classify accepted missing labels")
	}
	if err := s.Translate(nil, &TranslateRequest{Text: "x"}, &TranslateResponse{}); err == nil {
		t.Error("translate accepted a missing target language")
	}
	if err := s.Translate(nil, &TranslateRequest{Text: "x", To: strings.Repeat("l", 100)}, &TranslateResponse{}); err == nil {
		t.Error("translate accepted a sentence where a language belongs")
	}
}

// Nothing here is anybody's private data, and that is what lets a paying agent
// with no account use it. Scoping this service would close it to exactly the
// callers it exists for.
func TestServiceIsNotScoped(t *testing.T) {
	if Spec.Scoped {
		t.Error("text is scoped, which shuts out the anonymous paying callers it is for")
	}
	if len(Spec.Endpoints) != 4 {
		t.Errorf("expected four endpoints, got %d", len(Spec.Endpoints))
	}
	for name, ep := range Spec.Endpoints {
		if ep.Cost == "" {
			t.Errorf("%s has no price, but every call here costs us a model", name)
		}
		if ep.Needs == service.Account {
			t.Errorf("%s requires an account, so a paying agent cannot buy it", name)
		}
	}
}
