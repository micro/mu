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

func TestInterruptedFlowsAreClosedOnStartup(t *testing.T) {
	flows := []*Flow{
		{Status: "running", Steps: []FlowStep{{Status: "done"}, {Status: "running"}}},
		{Status: "done", Error: "keep me"},
	}

	if !reconcileInterruptedFlows(flows) {
		t.Fatal("running flow was not reconciled")
	}
	if flows[0].Status != "error" || flows[0].Error != interruptedFlowError {
		t.Errorf("interrupted flow = %+v", flows[0])
	}
	if flows[0].Steps[0].Status != "done" || flows[0].Steps[1].Status != "error" {
		t.Errorf("interrupted steps = %+v", flows[0].Steps)
	}
	if flows[1].Status != "done" || flows[1].Error != "keep me" {
		t.Errorf("completed flow changed = %+v", flows[1])
	}
	if reconcileInterruptedFlows(flows) {
		t.Error("reconciliation was not idempotent")
	}
}

func TestInterruptedFlowErrorIsVisibleToPendingClient(t *testing.T) {
	flow := &Flow{ID: "interrupted_pending_flow", AccountID: "interrupted_pending_account",
		ThreadID: "interrupted_pending_thread", Status: "error", Error: interruptedFlowError,
		CreatedAt: time.Now()}
	flowMu.Lock()
	flowStore[flow.ID] = flow
	flowMu.Unlock()
	t.Cleanup(func() {
		flowMu.Lock()
		delete(flowStore, flow.ID)
		flowMu.Unlock()
	})

	if got := flowError(flow.AccountID, flow.ThreadID); got != interruptedFlowError {
		t.Errorf("flowError = %q", got)
	}
}
