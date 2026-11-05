package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPIAnswerCarriesTypedResults(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"Content": `{"item":{"id":"abcdefghijk","title":"Selected video"}}`})
	items := resultItems(Step{Tool: "video_read", OK: true, Output: string(payload)})
	raw, err := json.Marshal(apiAnswer{Text: "Found it.", Thread: "owned-thread", Results: items})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Results []struct{ Kind, ID string }
		Thread  string
	}
	if err := json.Unmarshal(raw, &response); err != nil || len(response.Results) != 1 ||
		response.Results[0].Kind != "video" || response.Results[0].ID != "abcdefghijk" || response.Thread != "owned-thread" {
		t.Fatalf("selected video missing from response: %s (%v)", raw, err)
	}
	raw, _ = json.Marshal(apiAnswer{Text: "No video selected."})
	if strings.Contains(string(raw), "results") {
		t.Fatal("empty results should preserve the existing response shape")
	}
}
