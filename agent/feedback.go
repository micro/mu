package agent

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"mu/internal/app"
)

// ExecFeedback is the result of executing code in the browser.
type ExecFeedback struct {
	FlowID  string `json:"flow_id"`
	OK      bool   `json:"ok"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
	DOM     string `json:"dom,omitempty"` // text content of the preview
	ReceivedAt time.Time
}

var (
	feedbackMu    sync.Mutex
	feedbackStore = map[string]chan *ExecFeedback{} // flowID → channel
)

// waitForFeedback creates a channel and waits for the browser to send back
// the result of an exec. Returns nil on timeout.
func waitForFeedback(flowID string, timeout time.Duration) *ExecFeedback {
	ch := make(chan *ExecFeedback, 1)

	feedbackMu.Lock()
	feedbackStore[flowID] = ch
	feedbackMu.Unlock()

	defer func() {
		feedbackMu.Lock()
		delete(feedbackStore, flowID)
		feedbackMu.Unlock()
	}()

	select {
	case fb := <-ch:
		return fb
	case <-time.After(timeout):
		return nil
	}
}

// FeedbackHandler receives POST /agent/feedback from the browser after exec.
func FeedbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var fb ExecFeedback
	if err := json.NewDecoder(r.Body).Decode(&fb); err != nil {
		app.RespondJSON(w, map[string]string{"error": "invalid request"})
		return
	}
	fb.ReceivedAt = time.Now()

	feedbackMu.Lock()
	ch, ok := feedbackStore[fb.FlowID]
	feedbackMu.Unlock()

	if ok {
		select {
		case ch <- &fb:
		default:
		}
	}

	app.RespondJSON(w, map[string]string{"status": "ok"})
}
