package agent

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"mu/internal/app"
)

// ExecResult is the result of executing code in the browser.
type ExecResult struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	DOM       string `json:"dom,omitempty"`
}

var (
	execMu    sync.Mutex
	execStore = map[string]chan *ExecResult{}
)

// waitForExecResult waits for the browser to send back the result of an exec.
func waitForExecResult(sessionID string, timeout time.Duration) *ExecResult {
	ch := make(chan *ExecResult, 1)

	execMu.Lock()
	execStore[sessionID] = ch
	execMu.Unlock()

	defer func() {
		execMu.Lock()
		delete(execStore, sessionID)
		execMu.Unlock()
	}()

	select {
	case r := <-ch:
		return r
	case <-time.After(timeout):
		return nil
	}
}

// ExecResultHandler receives POST /agent/exec/result from the browser.
func ExecResultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var result ExecResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		app.RespondJSON(w, map[string]string{"error": "invalid request"})
		return
	}

	execMu.Lock()
	ch, ok := execStore[result.SessionID]
	execMu.Unlock()

	if ok {
		select {
		case ch <- &result:
		default:
		}
	}

	app.RespondJSON(w, map[string]string{"status": "ok"})
}
