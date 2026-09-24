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
	r := request{EventID: e.ID, Account: e.Account, ID: e.Resource}
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
	case event.ScheduleDue:
		var rsp events.ReadResponse
		if err := service.Call(ctx, "events", "Server.Read", &events.ReadRequest{ID: e.Resource}, &rsp); err != nil {
			return r, err
		}
		schedule := rsp.Item
		if schedule == nil || schedule.Paused || fmt.Sprint(schedule.Sequence) != e.Version {
			return request{}, nil
		}
		r.Kind, r.Title, r.Prompt = events.Kind, schedule.Title, strings.TrimSpace(schedule.Prompt)
		if schedule.Kind == "brief" {
			r.Prompt += "\nCover overnight developments and what matters today."
			if events.BriefWorldNews(schedule) {
				r.Prompt += "\nInclude a short world news section with current sources."
			} else {
				r.Prompt += "\nKeep this brief personal: calendar, messages, tasks and local weather. Do not fetch or include world news, general headlines, social trends or market news."
			}
		}
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
