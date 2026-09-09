package ai

import (
	gmagent "go-micro.dev/v6/agent"
	gmai "go-micro.dev/v6/ai"
	"testing"
)

func TestGeminiBuiltinPlanHasItems(t *testing.T) {
	tools, _ := gmagent.Builtins()
	before := &gmai.Request{Tools: tools}
	after := planSchema(before)
	found := false
	for i, tool := range after.Tools {
		if tool.Name != "plan" {
			continue
		}
		found = true
		steps := tool.Properties["steps"].(map[string]any)
		items, ok := steps["items"].(map[string]any)
		if !ok || items["type"] != "object" {
			t.Fatal("plan missing array items")
		}
		if before.Tools[i].Properties["steps"].(map[string]any)["items"] != nil {
			t.Fatal("shared tool mutated")
		}
	}
	if !found {
		t.Fatal("no built-in plan tested")
	}
}
