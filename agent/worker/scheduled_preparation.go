package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/persist"
	"mu/service/mail"
	"mu/service/tasks"
)

// A result belongs to one owner, schedule revision and delivery occurrence.
// Locks serialize preparation and delivery in this process; receipts survive restart.
var scheduledLocks [64]sync.Mutex

type preparedResult struct {
	MessageID string    `json:"message_id,omitempty"`
	State     string    `json:"state"`
	Answer    string    `json:"answer,omitempty"`
	Failure   string    `json:"failure,omitempty"`
	ReadyAt   time.Time `json:"ready_at,omitempty"`
}

func preparationKey(r request) (string, byte) {
	sum := sha256.Sum256([]byte(r.Account + "\x00" + r.ID + "\x00" + r.Revision + "\x00" + r.Due.UTC().Format(time.RFC3339Nano)))
	return "work/prepared/" + hex.EncodeToString(sum[:]) + ".json", sum[0] % 64
}

func consumePrepared(r request) error {
	return consumePreparedWith(r, agent.PrepareScheduled, agent.ScheduledCurrent, deliver)
}

func consumePreparedWith(r request, prepare func(string, string, string, time.Time) (bool, string, error), current func(string, string, string) bool, send func(request, string, error) error) error {
	key, slot := preparationKey(r)
	scheduledLocks[slot].Lock()
	defer scheduledLocks[slot].Unlock()
	var result preparedResult
	b, err := persist.Read(key)
	if err == nil {
		if err = json.Unmarshal(b, &result); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	syncTask := func() error {
		_, err := tasks.RecordOccurrence(r.Account, r.Title, r.Due, tasks.Occurrence{Key: key, Schedule: r.ID, Revision: r.Revision, State: result.State, Failure: result.Failure, MessageID: result.MessageID}, result.Answer)
		return err
	}
	save := func() error {
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := persist.Write(key, b); err != nil {
			return err
		}
		return syncTask()
	}
	if !current(r.Account, r.ID, r.Revision) {
		if result.State == "" {
			return nil
		}
		if result.State != "done" && result.State != "delivery_failed" {
			result.State = "canceled"
			return save()
		}
		return syncTask()
	}
	if result.State == "canceled" {
		return syncTask()
	}
	if result.State == "done" || result.State == "delivering" || result.State == "delivery_failed" {
		return syncTask()
	}
	if result.State == "" {
		release := tasks.BeginPreparation(r.Account, key)
		defer release()
		result.State = "started"
		if err := save(); err != nil {
			return err
		}
		_, result.Answer, err = prepare(r.Account, r.ID, r.Revision, r.Due)
		if err != nil {
			result.Failure = ai.FailureMessage(err)
		}
		result.State, result.ReadyAt = "ready", time.Now().UTC()
		if err := save(); err != nil {
			return err
		}
	} else if result.State == "started" {
		result.State, result.Failure = "ready", "Preparation was interrupted. It was not automatically repeated."
		if err := save(); err != nil {
			return err
		}
	}
	if r.Preparing {
		return syncTask()
	}
	if time.Now().Before(r.Due) {
		return fmt.Errorf("scheduled delivery is not due")
	}
	if !current(r.Account, r.ID, r.Revision) {
		result.State = "canceled"
		return save()
	}
	// An intentional no-op has no Inbox delivery. Keep the completed run, but
	// never reserve a message ID or call the sender for an empty result.
	if strings.TrimSpace(result.Answer) == "" && result.Failure == "" {
		result.State, result.MessageID = "done", ""
		return save()
	}
	// Reserve external delivery before sending: a restart must not duplicate it.
	result.State = "delivering"
	if r.EventID != "" {
		result.MessageID = "<schedule-" + r.EventID + "@" + mail.ConfiguredDomain() + ">"
	}
	if err := save(); err != nil {
		return err
	}
	var failure error
	if result.Failure != "" {
		failure = fmt.Errorf("%s", result.Failure)
	}
	if time.Since(r.Due) > time.Minute {
		app.Log("work", "scheduled delivery late: schedule=%s delay=%s", r.ID, time.Since(r.Due).Round(time.Second))
	}
	if err := send(r, result.Answer, failure); err != nil {
		result.State = "delivery_failed"
		result.Failure = ai.FailureMessage(err)
		return save()
	}
	result.State = "done"
	return save()
}
