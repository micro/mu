package work

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
	"mu/service/events"
	"mu/service/tasks"
)

func requestFor(e event.Record) (request, error) {
	r := request{EventID: e.ID, Revision: e.Version, Account: e.Account, ID: e.Resource}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = service.WithAccount(ctx, e.Account)
	switch e.Type {
	case event.TaskStarted:
		var rsp tasks.TaskResponse
		if err := service.Call(ctx, "tasks", "Server.Read", &tasks.DeleteRequest{ID: e.Resource}, &rsp); err != nil {
			return r, err
		}
		t := rsp.Item
		if t == nil || t.Status != tasks.StatusDoing || len(t.Attempts) == 0 || t.Attempts[len(t.Attempts)-1].ID != e.Version {
			return request{}, nil
		}
		r.Kind, r.Title, r.Thread, r.Agent = tasks.Kind, t.Title, t.Thread, t.Agent
		r.Prompt = t.Title
		if t.Detail != "" {
			r.Prompt += "\n\n" + t.Detail
		}
	case event.ScheduleDue, event.ScheduleAdvance:
		r.Preparing = e.Type == event.ScheduleAdvance
		if r.Preparing && e.Data["recipient"] != "agent" {
			return request{}, nil
		}
		switch when := e.Data["when"].(type) {
		case time.Time:
			r.Due = when
		case string:
			r.Due, _ = time.Parse(time.RFC3339Nano, when)
		}
		var schedule *events.Event
		for _, candidate := range events.List(e.Account) {
			if candidate.ID == e.Resource {
				schedule = candidate
				break
			}
		}
		if schedule == nil || schedule.Paused || fmt.Sprint(schedule.Sequence) != e.Version {
			return request{}, nil
		}
		r.Kind, r.Title, r.Prompt = events.Kind, schedule.Title, strings.TrimSpace(schedule.Prompt)
		r.Prepared = schedule.Advance.Recipient == "agent" && schedule.Advance.Minutes > 0
	}
	return r, nil
}

// A durable receipt prevents an event redelivery from repeating model/tool work.
// An interrupted run is reported for review, not automatically executed again.
func consumeWork(e event.Record) error {
	r, err := requestFor(e)
	if err != nil {
		return err
	}
	if r.Account != "" && r.Prompt != "" && r.Prepared {
		return consumePrepared(r)
	}
	return consumeRequest(r, run)
}

func consumeRequest(r request, execute func(request)) error {
	if r.Account == "" || r.Prompt == "" {
		return nil
	}
	key := "work/events/" + r.EventID + ".json"
	state, err := persist.Read(key)
	if err == nil {
		if string(state) == `"done"` {
			return nil
		}
		if r.Kind == events.Kind {
			deliver(r, "", fmt.Errorf("this scheduled run was interrupted; it was not automatically repeated"))
		}
		return persist.Write(key, []byte(`"done"`))
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := persist.Write(key, []byte(`"started"`)); err != nil {
		return err
	}
	execute(r)
	return persist.Write(key, []byte(`"done"`))
}
