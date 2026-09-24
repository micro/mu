package event

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/persist"
)

const (
	TaskStarted   = "tasks.started"
	ScheduleDue   = "events.due"
	ChatRecorded  = "chat.recorded"
	MailAccepted  = "mail.accepted"
	SMSVerified   = "sms.verified"
	ChatAddressed = "chat.addressed"
)

// Record is a fact about a committed service resource, not an agent command.
// Data carries small facts that cannot be fetched later (for example trust at
// arrival). Resource contents remain behind the owning service's interface.
type Record struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Service     string                 `json:"service"`
	Account     string                 `json:"account"`
	Resource    string                 `json:"resource"`
	Version     string                 `json:"version,omitempty"`
	Actor       string                 `json:"actor,omitempty"`
	Correlation string                 `json:"correlation,omitempty"`
	At          time.Time              `json:"at"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

var commitMu sync.Mutex
var consumersMu sync.Mutex
var consumers = map[string]bool{}

// Commit stores source mutations and events in the same recoverable commit.
// Callers hold their source store's lock until this returns.
func Commit(writes map[string][]byte, records ...Record) error {
	commitMu.Lock()
	defer commitMu.Unlock()
	sequence, err := lastSequence()
	if err != nil {
		return err
	}
	tx := make(map[string][]byte, len(writes)+len(records))
	for k, v := range writes {
		if strings.HasPrefix(k, "outbox/") {
			return fmt.Errorf("reserved outbox key")
		}
		tx[k] = v
	}
	for _, r := range records {
		if r.Type == "" || r.Service == "" || r.Account == "" || r.Resource == "" {
			return fmt.Errorf("event needs type, service, account and resource")
		}
		sequence++
		r.ID = fmt.Sprintf("%020d", sequence)
		if r.At.IsZero() {
			r.At = time.Now().UTC()
		}
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		tx["outbox/log/"+r.ID+".json"] = b
	}
	head, _ := json.Marshal(sequence)
	tx["outbox/head.json"] = head
	return persist.Batch(tx)
}

func validConsumer(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// Consume resumes from a durable checkpoint; callers retry after errors.
// A handler must be idempotent: a crash after handling but before checkpointing
// can deliver the same event again. A new consumer starts at the beginning.
func Consume(ctx context.Context, name string, topics []string, handle func(Record) error) error {
	if !validConsumer(name) {
		return fmt.Errorf("invalid consumer name")
	}
	consumersMu.Lock()
	if consumers[name] {
		consumersMu.Unlock()
		return fmt.Errorf("consumer %s is already running", name)
	}
	consumers[name] = true
	consumersMu.Unlock()
	defer func() {
		consumersMu.Lock()
		delete(consumers, name)
		consumersMu.Unlock()
	}()
	wanted := map[string]bool{}
	for _, t := range topics {
		wanted[t] = true
	}
	checkpoint := "outbox/consumers/" + name + ".json"
	cursor := ""
	if b, err := persist.Read(checkpoint); err == nil {
		if err := json.Unmarshal(b, &cursor); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		head, err := lastSequence()
		if err != nil {
			return err
		}
		var current int64
		if cursor != "" {
			current, err = strconv.ParseInt(cursor, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid consumer checkpoint: %w", err)
			}
		}
		for next := current + 1; next <= head; next++ {
			id := fmt.Sprintf("%020d", next)
			file := id + ".json"
			b, err := persist.Read("outbox/log/" + file)
			if err != nil {
				return err
			}
			var r Record
			if err = json.Unmarshal(b, &r); err != nil {
				return err
			}
			if wanted[r.Type] {
				if err = invoke(handle, r); err != nil {
					return fmt.Errorf("consumer %s event %s: %w", name, id, err)
				}
			}
			b, _ = json.Marshal(id)
			if err = persist.Write(checkpoint, b); err != nil {
				return err
			}
			cursor = id
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func invoke(handle func(Record) error, r Record) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("handler panicked")
		}
	}()
	return handle(r)
}

// The head is committed with the records, avoiding a full directory scan on
// every arrival and every consumer poll. The fallback reads older outboxes.
func lastSequence() (int64, error) {
	b, err := persist.Read("outbox/head.json")
	if err == nil {
		var n int64
		err = json.Unmarshal(b, &n)
		if err == nil && n < 0 {
			return 0, fmt.Errorf("invalid outbox head")
		}
		return n, err
	}
	if !os.IsNotExist(err) {
		return 0, err
	}
	names, err := persist.List("outbox/log")
	if err != nil || len(names) == 0 {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSuffix(names[len(names)-1], ".json"), 10, 64)
}
