package agent

// A safety net must not be woven from the thing it is catching.

import (
	"strings"
	"testing"
)

// The fallback that replaces a raw tool payload must not itself be one.
//
// This reached somebody. They asked a question in their inbox and got back
// `* {"files":[{"id":"ca9fa0fd-…","checksum":"492d5ea4…"` — because the model
// answered with the payload, the guard noticed and replaced it with a summary,
// and the summary was built by bulleting whatever lines the results held with
// no check that a line was prose. For a tool that answers in one JSON object
// the replacement was the same failure with a bullet in front of it.
func TestTheFallbackNeverHandsBackRawJSON(t *testing.T) {
	rag := []string{
		"files_list\n{\"files\":[{\"id\":\"ca9fa0fd-571f-4fa6-9aef-ba2b2d866046\"," +
			"\"name\":\"hello.csv\",\"size\":8,\"owner\":\"asim\"}]}",
		"recall_search\nNothing in your history mentions \"zip rendered properly\".",
	}

	got := completeToolAnswer(`{"files":[{"id":"ca9fa0fd"}]}`, rag)

	for _, never := range []string{`{"`, `"id":`, `"checksum":`, "ca9fa0fd"} {
		if strings.Contains(got, never) {
			t.Errorf("the answer carries raw tool output (%q):\n%s", never, got)
		}
	}
	if strings.TrimSpace(got) == "" {
		t.Error("dropping the payload left nothing at all — an empty answer reads as a broken page")
	}
}

// A sentence in a JSON wrapper keeps the sentence and loses the wrapper.
//
// The first cut of the filter dropped these, which traded one failure for a
// worse one: plenty of tools answer with prose in a field, and refusing to read
// it throws the answer away to avoid showing the braces around it.
func TestASentenceInAWrapperSurvivesTheWrapper(t *testing.T) {
	got := completeToolAnswer("Let me pull the latest for you.", []string{
		"weather_forecast\n{\"summary\":\"Weather for London. Now: 14C, light rain.\"}",
	})
	if !strings.Contains(got, "14C") {
		t.Errorf("the answer inside the wrapper was thrown away:\n%s", got)
	}
	if strings.Contains(got, `{"`) || strings.Contains(got, `"summary"`) {
		t.Errorf("the wrapper came through with it:\n%s", got)
	}

	// And a record set has no sentence in it, so it stays dropped.
	if got := readableFromPayload(`{"files":[{"id":"abc","name":"x.csv"}]}`); got != "" {
		t.Errorf("a list of records was read as prose: %q", got)
	}
}

// And prose is not thrown away with it. Dropping a real answer is the worse
// error, so the test is one-sided on purpose.
func TestProseSurvivesThePayloadFilter(t *testing.T) {
	for _, keep := range []string{
		"Bitcoin is trading at $64,200, up 2.1% on the day.",
		"The file hello.csv is 8 bytes and was uploaded by you.",
		"Rain from about 3pm, 14°C — worth a coat.",
		"See https://example.com/a?x={y} for the details.",
	} {
		if isPayloadLine(keep) {
			t.Errorf("a sentence was taken for a payload: %q", keep)
		}
	}
	for _, drop := range []string{
		`{"files":[{"id":"abc","name":"x.csv"}]}`,
		`[{"title":"a","url":"b"}]`,
		`"id":"a","name":"b","size":8,"owner":"c"`,
	} {
		if !isPayloadLine(drop) {
			t.Errorf("a payload was taken for a sentence: %q", drop)
		}
	}
}
