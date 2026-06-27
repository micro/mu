package a2a

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentCardHandlerUsesConfiguredBaseURLAndListsSkills(t *testing.T) {
	oldBaseURL := BaseURL
	BaseURL = "https://mu.example"
	t.Cleanup(func() { BaseURL = oldBaseURL })

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	rr := httptest.NewRecorder()

	AgentCardHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("AgentCardHandler status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var card AgentCard
	if err := json.NewDecoder(rr.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card: %v", err)
	}
	if len(card.SupportedInterfaces) != 1 {
		t.Fatalf("SupportedInterfaces length = %d, want 1", len(card.SupportedInterfaces))
	}
	if got, want := card.SupportedInterfaces[0].URL, "https://mu.example/a2a"; got != want {
		t.Fatalf("interface URL = %q, want %q", got, want)
	}
	if len(card.Skills) == 0 {
		t.Fatal("expected advertised skills")
	}
	for _, skill := range card.Skills {
		if skill.ID == "micro" {
			t.Fatal("agent card should not advertise the fallback micro agent as a skill")
		}
	}
}

func TestHandlerRejectsNonPOST(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/a2a", nil)
	rr := httptest.NewRecorder()

	Handler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Handler status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerReturnsParseErrorForInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	Handler(rr, req)

	assertRPCError(t, rr, nil, -32700, "Parse error")
}

func TestHandlerReturnsMethodNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(`{"jsonrpc":"2.0","id":"req-1","method":"Nope"}`))
	rr := httptest.NewRecorder()

	Handler(rr, req)

	assertRPCError(t, rr, "req-1", -32601, "Method not found: Nope")
}

func TestGetTaskAndCancelTaskReturnNotFoundForMissingTask(t *testing.T) {
	resetTasks(t)

	for _, method := range []string{"GetTask", "CancelTask"} {
		t.Run(method, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":"req-1","method":"` + method + `","params":{"id":"missing"}}`
			req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
			rr := httptest.NewRecorder()

			Handler(rr, req)

			assertRPCError(t, rr, "req-1", -32001, "Task not found")
		})
	}
}

func TestCancelTaskUpdatesStoredTask(t *testing.T) {
	resetTasks(t)
	taskMu.Lock()
	tasks["task-1"] = &Task{ID: "task-1", Status: TaskStatus{State: "TASK_STATE_WORKING"}}
	taskMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(`{"jsonrpc":"2.0","id":"req-1","method":"CancelTask","params":{"id":"task-1"}}`))
	rr := httptest.NewRecorder()

	Handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Handler status = %d, want %d", rr.Code, http.StatusOK)
	}
	var resp rpcResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	encoded, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var got Task
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode result task: %v", err)
	}
	if got.Status.State != "TASK_STATE_CANCELED" {
		t.Fatalf("task state = %q, want TASK_STATE_CANCELED", got.Status.State)
	}
	if got.Status.Timestamp == "" {
		t.Fatal("expected cancel timestamp")
	}
}

func TestAgentForSkill(t *testing.T) {
	got := AgentForSkill("weather")
	if !strings.Contains(got, "Weather Agent") || !strings.Contains(got, "Weather forecasts") {
		t.Fatalf("AgentForSkill(weather) = %q, want weather agent description", got)
	}
	if got := AgentForSkill("missing"); got != "" {
		t.Fatalf("AgentForSkill(missing) = %q, want empty string", got)
	}
}

func resetTasks(t *testing.T) {
	t.Helper()
	taskMu.Lock()
	oldTasks := tasks
	tasks = map[string]*Task{}
	taskMu.Unlock()
	t.Cleanup(func() {
		taskMu.Lock()
		tasks = oldTasks
		taskMu.Unlock()
	})
}

func assertRPCError(t *testing.T, rr *httptest.ResponseRecorder, wantID any, wantCode int, wantMessage string) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("Handler status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var resp rpcResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatalf("JSONRPC = %q, want 2.0", resp.JSONRPC)
	}
	if resp.ID != wantID {
		t.Fatalf("ID = %#v, want %#v", resp.ID, wantID)
	}
	if resp.Error == nil {
		t.Fatal("expected RPC error")
	}
	if resp.Error.Code != wantCode || resp.Error.Message != wantMessage {
		t.Fatalf("error = (%d, %q), want (%d, %q)", resp.Error.Code, resp.Error.Message, wantCode, wantMessage)
	}
}
