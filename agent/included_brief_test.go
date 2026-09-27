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
