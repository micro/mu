package tasks

import (
	"fmt"
	"mu/internal/userdb"
	"time"
)

// Occurrence is a scheduled execution, not another independently runnable task.
// Schedule identifies its group; Key distinguishes revisions and due times.
type Occurrence struct {
	Key       string `json:"key"`
	Schedule  string `json:"schedule"`
	Revision  string `json:"revision"`
	Execution string `json:"execution"`
	Delivery  string `json:"delivery"`
	State     string `json:"state"`
	Failure   string `json:"failure,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

func RecordOccurrence(owner, title string, due time.Time, o Occurrence, result string) (*Task, error) {
	if owner == "" || o.Key == "" || o.Schedule == "" || due.IsZero() {
		return nil, fmt.Errorf("owned schedule occurrence required")
	}
	runMu.Lock()
	defer runMu.Unlock()
	records, err := userdb.List(ns, owner, collection, "mine", map[string]any{"occurrence_key": o.Key}, "", "", 1)
	if err != nil {
		return nil, err
	}
	execution, delivery := "completed", "waiting"
	if len(records) > 0 {
		if previous := decodeOccurrence(records[0].Data); previous != nil {
			execution, delivery = previous.Execution, previous.Delivery
		}
	}
	switch o.State {
	case "started":
		execution, delivery = "running", "waiting"
	case "ready":
		execution, delivery = "completed", "pending"
		if o.Failure != "" {
			execution = "failed"
		}
	case "delivering":
		delivery = "sending"
	case "delivery_failed":
		delivery = "failed"
	case "done":
		delivery = "delivered"
	case "canceled":
		delivery = "canceled"
		if execution == "running" {
			execution = "canceled"
		}
	}
	at := now()
	fields := map[string]any{"title": title, "assignee": Agent, "created": stamp(at), "updated": stamp(at), "due": stamp(due), "schedule_id": o.Schedule, "occurrence_key": o.Key, "occurrence_revision": o.Revision, "occurrence_state": o.State, "occurrence_execution": execution, "occurrence_delivery": delivery, "occurrence_failure": o.Failure, "occurrence_message": o.MessageID, "result": result}
	status := StatusDone
	switch o.State {
	case "started":
		status = StatusDoing
	case "canceled":
		status = StatusCanceled
	case "delivery_failed":
		status = StatusFailed
	}
	if o.Failure != "" && o.State != "canceled" {
		status = StatusFailed
	}
	fields["status"] = status
	attemptStatus := StatusDone
	if execution == "running" {
		attemptStatus = StatusDoing
	} else if execution == "failed" {
		attemptStatus = StatusFailed
	} else if execution == "canceled" {
		attemptStatus = StatusCanceled
	}
	attempt := Attempt{ID: o.Key, Started: at, Status: attemptStatus, Report: result, Error: o.Failure}
	if len(records) > 0 {
		old := toTask(records[0].ID, owner, records[0].Data)
		fields["created"] = stamp(old.Created)
		if len(old.Attempts) > 0 {
			attempt.Started = old.Attempts[0].Started
			attempt.Finished = old.Attempts[0].Finished
		}
	}
	if o.State != "started" && attempt.Finished.IsZero() {
		attempt.Finished = at
	}
	fields["attempts"] = encodeAttempts([]Attempt{attempt})
	var rec *userdb.Record
	if len(records) == 0 {
		rec, err = userdb.Create(ns, owner, collection, fields, false)
	} else {
		rec, err = userdb.Update(ns, owner, collection, records[0].ID, fields, false)
	}
	if err != nil {
		return nil, err
	}
	t := toTask(rec.ID, owner, rec.Data)
	index(t)
	return t, nil
}

// LatestOccurrences groups before limiting, so daily runs cannot crowd out groups.
func LatestOccurrences(owner string) []*Task {
	records, err := userdb.LatestBy(ns, owner, collection, []string{"schedule_id"}, "due", userdb.MaxListLimit)
	if err != nil {
		return nil
	}
	var out []*Task
	for _, r := range records {
		t := toTask(r.ID, owner, r.Data)
		if t.Occurrence != nil {
			out = append(out, t)
		}
	}
	return out
}

func Occurrences(owner, schedule string) []*Task {
	if owner == "" || schedule == "" {
		return nil
	}
	records, err := userdb.List(ns, owner, collection, "mine", map[string]any{"schedule_id": schedule}, "due", "desc", userdb.MaxListLimit)
	if err != nil {
		return nil
	}
	var out []*Task
	for _, r := range records {
		out = append(out, toTask(r.ID, owner, r.Data))
	}
	return out
}

func decodeOccurrence(d map[string]any) *Occurrence {
	value := func(k string) string { s, _ := d[k].(string); return s }
	if value("schedule_id") == "" {
		return nil
	}
	return &Occurrence{Key: value("occurrence_key"), Schedule: value("schedule_id"), Revision: value("occurrence_revision"), State: value("occurrence_state"), Execution: value("occurrence_execution"), Delivery: value("occurrence_delivery"), Failure: value("occurrence_failure"), MessageID: value("occurrence_message")}
}
