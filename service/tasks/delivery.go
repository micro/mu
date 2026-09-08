package tasks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"mu/internal/userdb"
)

// Delivery is a persisted outcome awaiting acknowledgement by its consumer.
// The service stores the payload; it does not choose a transport or send it.
type Delivery struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	From string `json:"from"`
}

func encodeDelivery(d *Delivery) string {
	b, _ := json.Marshal(d)
	return string(b)
}

func decodeDelivery(s string) *Delivery {
	var d Delivery
	if json.Unmarshal([]byte(s), &d) != nil || d.ID == "" {
		return nil
	}
	return &d
}

// RecordOutcome saves the result and pending reply in one store write.
func RecordOutcome(owner, id, status, result, reply, from string, steps []Step) (*Task, error) {
	runMu.Lock()
	defer runMu.Unlock()
	t, err := Get(owner, id)
	if err != nil {
		return nil, err
	}
	if t.Delivery != nil {
		return nil, fmt.Errorf("a result is already awaiting delivery")
	}
	extra := map[string]any{}
	if t.Thread != "" && strings.TrimSpace(reply) != "" {
		delivery := &Delivery{ID: uuid.NewString(), Text: reply, From: from}
		extra["delivery"] = encodeDelivery(delivery)
		extra["delivery_pending"] = true
		extra["delivery_id"] = delivery.ID
	}
	return update(owner, id, "", "", status, "", result, extra, steps)
}

// PendingDeliveries returns a bounded batch; acknowledged items leave the set.
func PendingDeliveries(owner, after string) []*Task {
	if owner == "" {
		return nil
	}
	where := map[string]any{"delivery_pending": true}
	if after != "" {
		where["delivery_id"] = map[string]any{"gt": after}
	}
	records, err := userdb.List(ns, owner, collection, "mine", where, "delivery_id", "asc", userdb.MaxListLimit)
	if err != nil {
		return nil
	}
	var out []*Task
	for _, rec := range records {
		if t := toTask(rec.ID, rec.Owner, rec.Data); t.Delivery != nil {
			out = append(out, t)
		}
	}
	return out
}

// AcknowledgeDelivery cannot clear a different, newer outcome.
func AcknowledgeDelivery(owner, id, deliveryID string) error {
	runMu.Lock()
	defer runMu.Unlock()
	t, err := Get(owner, id)
	if err != nil {
		return err
	}
	if t.Delivery == nil {
		return nil
	}
	if t.Delivery.ID != deliveryID {
		return fmt.Errorf("the pending delivery has changed")
	}
	_, err = update(owner, id, "", "", "", "", "", map[string]any{"delivery": nil, "delivery_pending": nil, "delivery_id": nil})
	return err
}
