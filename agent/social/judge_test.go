package social

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPicksIsForgivingAboutShapeAndStrictAboutContent. A model that answers
// "1, 3" and one that answers "I'd go with 1 and 3." mean the same thing, and
// neither should be able to publish a post that was not on the list.
func TestPicksIsForgivingAboutShapeAndStrictAboutContent(t *testing.T) {
	for _, c := range []struct {
		answer string
		want   []int
	}{
		{"2,1", []int{1, 0}},
		{"2, 1", []int{1, 0}},
		{"I'd go with 2 and 1.", []int{1, 0}},
		{"2\n1\n", []int{1, 0}},
		{"", nil},
		{"none", nil},
		{"9, 40, 0", nil},             // out of range, silently dropped
		{"1, 1, 1", []int{0}},         // deduped
		{"1,2,3,4,5", []int{0, 1, 2}}, // never more than a review may post
	} {
		got := picks(c.answer, 4)
		if len(got) != len(c.want) {
			t.Errorf("picks(%q) = %v, want %v", c.answer, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("picks(%q) = %v, want %v", c.answer, got, c.want)
				break
			}
		}
	}
}

// TestTheModelsOrderIsKept — the judge is being asked which is best, so its
// order is its answer, not a re-run of the score.
func TestTheModelsOrderIsKept(t *testing.T) {
	got := picks("3,1", 3)
	if len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Errorf("picks kept %v rather than the model's order", got)
	}
}

// TestWithoutAModelTheShortlistIsStillPublished — an instance with no LLM
// configured is a supported instance, and social should still work on it.
func TestWithoutAModelTheShortlistIsStillPublished(t *testing.T) {
	var got []*candidate
	Surface = func(c *candidate) { got = append(got, c) }
	defer func() { Surface = func(*candidate) {} }()

	candidates, surfaced = nil, map[string]bool{}
	for _, c := range []*candidate{
		{DID: "a", Category: "Tech", Link: "https://a", Score: 10},
		{DID: "b", Category: "Dev", Link: "https://b", Score: 90},
		{DID: "c", Category: "UK", Link: "https://c", Score: 50},
	} {
		candidates = append(candidates, c)
	}
	review() // no AI configured in a test, so judge returns nothing

	if len(got) != 3 {
		t.Fatalf("surfaced %d with no model configured, want 3", len(got))
	}
	if got[0].Link != "https://b" {
		t.Errorf("fell back to arrival order rather than score: %s first", got[0].Link)
	}
}

// TestTheShortlistIsWhatTheModelSees — best first, one per category, one per
// author. Each cap is isolated here so a change that quietly drops one is
// visible: two of the five below are refused for one reason each.
func TestTheShortlistIsWhatTheModelSees(t *testing.T) {
	short := shortlist([]*candidate{
		{DID: "bot", Category: "Dev", Link: "https://keep-1", Score: 90},
		{DID: "someone", Category: "Tech", Link: "https://keep-2", Score: 80},
		{DID: "bot", Category: "UK", Link: "https://same-author", Score: 70},
		{DID: "person", Category: "Tech", Link: "https://same-category", Score: 60},
		{DID: "person", Category: "World", Link: "https://keep-3", Score: 40},
	})
	var got []string
	for _, c := range short {
		got = append(got, strings.TrimPrefix(c.Link, "https://"))
	}
	want := "keep-1 keep-2 keep-3"
	if strings.Join(got, " ") != want {
		t.Errorf("shortlist = %q, want %q", strings.Join(got, " "), want)
	}
}

// TestTheModelIsShownEnoughToChooseAndNoMore.
func TestTheModelIsShownEnoughToChooseAndNoMore(t *testing.T) {
	long := strings.Repeat("a considered sentence about compilers. ", 40)
	out := numbered([]*candidate{
		{Category: "Dev", Link: "https://blog.example.com/x", Text: long},
		{Category: "Tech", Text: "no link, just an image"},
	})
	if !strings.Contains(out, "1. [Dev] (blog.example.com)") {
		t.Errorf("the model is not told the category and the source:\n%s", out)
	}
	if !strings.Contains(out, "(no link)") {
		t.Error("a post with no link should say so rather than show an empty bracket")
	}
	if len(out) > 2*judgeChars {
		t.Errorf("two posts rendered to %d characters", len(out))
	}
}

// ── The fast path ───────────────────────────────────────────────

// TestTheFastPathRefusesWhatTheRealFilterRefuses. worthParsing exists so that
// 84% of the firehose never reaches the JSON decoder, which means it decides
// on raw bytes what consider decides on a parsed event. It is only allowed to
// be wrong in one direction: never drop something consider would have kept.
func TestTheFastPathRefusesWhatTheRealFilterRefuses(t *testing.T) {
	raw := func(ev event, rec map[string]any) []byte {
		b, _ := json.Marshal(map[string]any{
			"did": ev.DID, "kind": "commit", "time_us": 1786527580809679,
			"commit": map[string]any{
				"operation": ev.Commit.Operation, "collection": "app.bsky.feed.post",
				"record": rec,
			},
		})
		return b
	}
	base := map[string]any{"$type": "app.bsky.feed.post", "text": techPost, "langs": []string{"en"}}
	create := event{DID: "x"}
	create.Commit.Operation = "create"
	del := event{DID: "x"}
	del.Commit.Operation = "delete"

	if !worthParsing(raw(create, base)) {
		t.Error("the fast path dropped an English create")
	}
	if worthParsing(raw(del, base)) {
		t.Error("the fast path let a delete through")
	}

	ja := map[string]any{"text": techPost, "langs": []string{"ja"}}
	if worthParsing(raw(create, ja)) {
		t.Error("the fast path let a language nobody here can read through")
	}
	// A regional tag is still English.
	for _, langs := range [][]string{{"en-GB"}, {"ja", "en"}, {"en", "fr"}} {
		rec := map[string]any{"text": techPost, "langs": langs}
		if !worthParsing(raw(create, rec)) {
			t.Errorf("the fast path dropped langs %v", langs)
		}
	}
	// No language tag at all.
	if worthParsing(raw(create, map[string]any{"text": techPost})) {
		t.Error("the fast path let an untagged post through")
	}

	reply := map[string]any{"text": techPost, "langs": []string{"en"},
		"reply": map[string]any{"root": map[string]any{"uri": "at://x"}}}
	if worthParsing(raw(create, reply)) {
		t.Error("the fast path let a reply through")
	}
}
