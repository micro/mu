package agent

import "testing"

func TestBriefStructuredResponse(t *testing.T) {
	for _, bad := range []string{"Good morning!", "null", "{}", `{"day":"a paragraph"}`, `{"day":[]} trailing`} {
		if _, err := parseBriefContent(bad); err == nil {
			t.Fatalf("accepted invalid response %s", bad)
		}
	}
	result, err := parseBriefContent(`{"day":[{"text":"Meeting at 9am","url":"https://micro.mu/events"}],"weather":[],"prayer":[],"headlines":[],"priorities":[]}`)
	if err != nil || len(result.Day) != 1 {
		t.Fatal(result, err)
	}
}

func TestBriefUsesApplicationMarketsBeforeValidation(t *testing.T) {
	quote := BriefItem{Title: "BTC", Text: "$77775.94 (+1.79%)"}
	for _, raw := range []string{
		`{"day":[],"travel":[],"weather":[],"prayer":[],"headlines":[],"markets":[],"priorities":[]}`,
		`{"markets":[{"text":"Invented model prices"}]}`,
	} {
		result, err := parseBriefContent(raw, quote)
		if err != nil || len(result.Markets) != 1 || result.Markets[0] != quote {
			t.Fatalf("application markets not used: %+v, %v", result, err)
		}
	}
	for _, raw := range []string{"null", "not JSON", `{"day":"invalid"}`} {
		if _, err := parseBriefContent(raw, quote); err == nil {
			t.Fatalf("accepted malformed response with market fallback: %s", raw)
		}
	}
	if _, err := parseBriefContent(`{"markets":[{"text":"Invented model prices"}]}`); err == nil {
		t.Fatal("model market prices counted as usable content")
	}
}
