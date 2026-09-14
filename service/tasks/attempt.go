package tasks

import (
	"encoding/json"
	"fmt"
	"time"
)

// Attempt preserves what happened on each execution, including unsuccessful reporting.
type Attempt struct {
	ID       string    `json:"id"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
	Version  string    `json:"version,omitempty"`
	Status   string    `json:"status"`
	Steps    []Step    `json:"steps,omitempty"`
	Report   string    `json:"report,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func encodeAttempts(a []Attempt) string { b, _ := json.Marshal(a); return string(b) }
func decodeAttempts(s string) []Attempt { var a []Attempt; _ = json.Unmarshal([]byte(s), &a); return a }

// Progress replaces the current attempt snapshot, refusing late callbacks from older attempts.
func Progress(owner, id, run string, steps []Step, report, failure, version string) error {
	runMu.Lock()
	defer runMu.Unlock()
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if len(t.Attempts) == 0 || t.Attempts[len(t.Attempts)-1].ID != run || t.Status != StatusDoing {
		return fmt.Errorf("run is no longer active")
	}
	a := &t.Attempts[len(t.Attempts)-1]
	a.Steps = steps
	a.Report = report
	a.Error = failure
	if version != "" {
		a.Version = version
	}
	_, err = update(owner, id, "", "", "", "", "", map[string]any{"attempts": encodeAttempts(t.Attempts)}, steps)
	return err
}

func ArchiveTask(owner, id string, archive bool) error {
	runMu.Lock()
	defer runMu.Unlock()
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if t.Status == StatusDoing {
		return fmt.Errorf("stop the running task first")
	}
	_, err = update(owner, id, "", "", "", "", "", map[string]any{"archived": archive})
	return err
}

// Stop persists the stop before the worker is canceled, so late results cannot replace it.
func Stop(owner, id string) error {
	runMu.Lock()
	defer runMu.Unlock()
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if t.Status != StatusDoing && t.Status != StatusTodo {
		return fmt.Errorf("task is not running or queued")
	}
	extra := map[string]any{}
	if n := len(t.Attempts); n > 0 {
		t.Attempts[n-1].Status = StatusCanceled
		t.Attempts[n-1].Finished = now()
		extra["attempts"] = encodeAttempts(t.Attempts)
	}
	_, err = update(owner, id, "", "", StatusCanceled, "", "Stopped. Actions already completed are retained in the activity report.", extra)
	return err
}
