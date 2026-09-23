package agent

import (
	"encoding/json"
	"testing"
)

func TestVideoCandidatesDoNotBecomePlayers(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"Content": `{"items":[{"id":"abcdefghijk","title":"Candidate"}]}`})
	for _, tool := range []string{"video_search", "video_list"} {
		if got := resultItems(Step{Tool: tool, OK: true, Output: string(payload)}); len(got) != 0 {
			t.Fatalf("unselected candidate embedded by %s", tool)
		}
	}
	payload, _ = json.Marshal(map[string]string{"Content": `{"item":{"id":"abcdefghijk","title":"Selected video"}}`})
	got := resultItems(Step{Tool: "video_read", OK: true, Output: string(payload)})
	if len(got) != 1 || got[0].Kind != "video" || got[0].ID != "abcdefghijk" {
		t.Fatalf("selected video missing: %+v", got)
	}
}

func TestAppResultsRequireSuccessfulTypedOutput(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"Content": `{"item":{"kind":"app","id":"my-widget","title":"My widget"}}`})
	for _, tool := range []string{"apps_create", "apps_edit", "apps_read", "apps_build", "apps_buildstatus"} {
		s := Step{Tool: tool, OK: true, Output: string(payload)}
		if got := resultItems(s); len(got) != 1 || got[0].ID != "my-widget" {
			t.Fatalf("missing app result from %s", tool)
		}
		s.OK = false
		if len(resultItems(s)) != 0 {
			t.Fatal("failed tool embedded")
		}
	}
	if len(resultItems(Step{Tool: "apps_build", OK: true, Output: `{"Content":"{\"state\":\"queued\"}"}`})) != 0 {
		t.Fatal("queued app embedded")
	}
}

func TestDocumentsBecomeBoundedPreviewResults(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"doc": map[string]string{"id": "doc-id", "title": "A doc", "content": "First\nSecond"}})
	envelope, _ := json.Marshal(map[string]string{"Content": string(raw)})
	got := resultItems(Step{Tool: "docs_write", OK: true, Output: string(envelope)})
	if len(got) != 1 || got[0].Kind != "doc" || got[0].Body != "First\nSecond" {
		t.Fatalf("missing doc preview: %+v", got)
	}
}
