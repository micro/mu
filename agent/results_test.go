package agent

import (
	"encoding/json"
	gmai "go-micro.dev/v6/model"
	"mu/internal/app"
	"mu/internal/result"
	"mu/internal/thread"
	"mu/service/news"
	"mu/service/video"
	"strings"
	"testing"
)

func presentationStep(t *testing.T, name string, response any) Step {
	t.Helper()
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(gmai.ToolResult{Content: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	return Step{Tool: name, OK: true, Output: string(out)}
}
func TestToolsProduceDurableConversationResults(t *testing.T) {
	v := presentationStep(t, "video_search", video.SearchResponse{Items: []*video.Result{{ID: "arabic_fruits", Title: "Arabic fruits"}}})
	items := resultItems(v)
	if len(items) != 1 || items[0].Kind != "video" || !strings.Contains(app.Results(items), "youtube.com/embed/arabic_fruits") {
		t.Fatalf("video is not playable: %+v", items)
	}
	v.OK = false
	if len(resultItems(v)) != 0 {
		t.Fatal("failed tools produced results")
	}
	n := presentationStep(t, "news_headlines", news.HeadlinesResponse{Items: []news.Headline{{Title: "A headline", URL: "https://example.com/story", Description: "Summary"}}})
	items = mergeResults(items, resultItems(n))
	n = presentationStep(t, "news_search", news.SearchResponse{Text: `{"results":[{"title":"Read this headline","url":"https://example.com/story","description":"New summary"}]}`})
	items = mergeResults(items, resultItems(n))
	if len(items) != 2 || items[1].Summary != "New summary" {
		t.Fatalf("search wrapper or deduplication: %+v", items)
	}
	id := Opened("result_owner", thread.WebClient, "persisted-results", "", "")
	Answered("result_owner", id, "Here it is", "expired-run", items...)
	msgs := thread.Messages("result_owner", id, 10)
	if len(msgs) != 1 || len(msgs[0].Results) != 2 {
		t.Fatalf("results lost from record: %+v", msgs)
	}
	html := renderTurn(msgs[0], "Micro")
	if !strings.Contains(html, "youtube.com/embed/arabic_fruits") || !strings.Contains(html, "data-save-url=") {
		t.Fatal("reopened turn lost player or save action")
	}
	for i := 0; i < 10; i++ {
		items = mergeResults(items, []result.Item{{Title: "Result", URL: strings.Repeat("x", i+1)}})
	}
	if len(items) != 6 {
		t.Fatalf("unbounded results: %d", len(items))
	}
}

func TestManagementToolsRespectTheCallingScope(t *testing.T) {
	for _, tc := range []struct {
		owner string
		opts  QueryOpts
	}{{"", QueryOpts{}}, {"owner", QueryOpts{Public: true}}, {"owner", QueryOpts{Tools: []string{"news"}}}} {
		if len(managementTools(tc.owner, tc.opts)) != 0 {
			t.Fatal("management exposed to guest or specialist")
		}
	}
	if len(managementTools("owner", QueryOpts{})) != 2 {
		t.Fatal("general agent cannot manage focused agents")
	}
	if _, err := createFocusedAgent("owner", map[string]any{"name": "reader", "prompt": "Read the news", "services": []any{}}); err == nil {
		t.Fatal("empty tool scope would grant broad access")
	}
	if _, err := createFocusedAgent("owner", map[string]any{"name": "reader", "prompt": "Read the news", "services": []any{"does_not_exist"}}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}
