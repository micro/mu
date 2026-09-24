package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/persist"
)

// WatchEvents connects committed service facts to agent-owned response policy.
// prepare reads through the service interface and checks current authorization.
// Lookup failures are retryable; model/tool execution is reserved before it starts
// and is never automatically repeated after a crash or uncertain outcome.
func WatchEvents(name string, topics []string, prepare func(event.Record) (func(), error)) {
	go func() {
		for {
			err := event.Consume(context.Background(), name, topics, func(e event.Record) error {
				acc, err := auth.GetAccount(e.Account)
				if err != nil || acc == nil || acc.Banned {
					return nil
				}
				return reactOnce(name, e, prepare)
			})
			app.Log("agent", "%s event consumer stopped: %v", name, err)
			time.Sleep(5 * time.Second)
		}
	}()
}

func reactOnce(name string, e event.Record, prepare func(event.Record) (func(), error)) error {
	if e.ID == "" || strings.Trim(e.ID, "0123456789") != "" {
		return fmt.Errorf("invalid event id")
	}
	key := "agent/events/" + name + "/" + e.ID + ".json"
	if b, err := persist.Read(key); err == nil {
		var state string
		if err := json.Unmarshal(b, &state); err != nil {
			return err
		}
		switch state {
		case "done", "interrupted":
			return nil
		case "started":
			app.Log("agent", "%s event %s for account %s was interrupted; review before retrying", name, e.ID, e.Account)
			return persist.Write(key, []byte(`"interrupted"`))
		default:
			return fmt.Errorf("invalid agent event receipt")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	execute, err := prepare(e)
	if err != nil {
		return err
	}
	if execute == nil {
		return nil
	}
	if err := persist.Write(key, []byte(`"started"`)); err != nil {
		return err
	}
	execute()
	return persist.Write(key, []byte(`"done"`))
}
