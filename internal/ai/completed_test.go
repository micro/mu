package ai

import (
	model "go-micro.dev/v6/model"
	"testing"
)

func TestCompletedTextRejectsNonFinalResponses(t *testing.T) {
	for _, response := range []*model.Response{nil, {}, {Reply: "draft", StopReason: "length"}, {Reply: "planning", ToolCalls: []model.ToolCall{{Name: "news_read"}}}} {
		if _, err := completedText(response); err == nil {
			t.Fatal("accepted unfinished response")
		}
	}
	if got, err := completedText(&model.Response{Reply: "Final answer", StopReason: "stop"}); err != nil || got != "Final answer" {
		t.Fatal(got, err)
	}
}
