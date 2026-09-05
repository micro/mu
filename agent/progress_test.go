package agent

import (
	"strings"
	"testing"
	"time"
)

func TestRunningFlowProgressIsFoundByConversation(t *testing.T) {
	const account = "progress_account"
	const threadID = "progress_thread"
	flow := &Flow{
		ID: "progress_flow", AccountID: account, ThreadID: threadID,
		Status: "running", CreatedAt: time.Now(),
		Steps: []FlowStep{{
			ID: "call_1", Tool: "web_search", Label: "Searching the web", Status: "running",
		}},
	}
	flowMu.Lock()
	flowStore[flow.ID] = flow
	flowMu.Unlock()
	t.Cleanup(func() {
		flowMu.Lock()
		delete(flowStore, flow.ID)
		flowMu.Unlock()
	})

	got := flowProgress(account, threadID)
	if len(got) != 1 || got[0].Label != "Searching the web" {
		t.Fatalf("progress = %+v", got)
	}
	got[0].Label = "changed"
	if flow.Steps[0].Label == "changed" {
		t.Error("flowProgress returned the store's mutable slice")
	}
}

func TestWebToolProgressIsPersistedWhileRunning(t *testing.T) {
	src := webSource(t)
	for _, want := range []string{
		`Status: "running"`,
		`f.Steps[i].Status = "done"`,
		"updateFlow(flow.ID",
		"flow.ThreadID = threadID",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("live flow progress is missing %q", want)
		}
	}
}
