package mail

// An accepted send owns its transport retries. The model must not repeat its
// work because SMTP is unavailable. Store the exact signed bytes and progress
// per recipient; an uncertain SMTP acknowledgement may still cause a duplicate
// at the far end, but retries retain the same Message-ID and MIME content.

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/userdb"
)

const outboxCollection = "outbox"

var outboxWake = make(chan struct{}, 1)
var outboxRunMu sync.Mutex

type queuedMail struct {
	From       string          `json:"from"`
	Subject    string          `json:"subject"`
	MessageID  string          `json:"message_id"`
	Message    []byte          `json:"message"`
	Recipients []string        `json:"recipients"`
	Delivered  map[string]bool `json:"delivered"`
	Attempts   int             `json:"attempts"`
	Created    time.Time       `json:"created"`
	LastError  string          `json:"last_error,omitempty"`
}

func queueReply(owner, display, from, to string, cc []string, subject, plain, html, parent, refs string) (string, error) {
	message, id := buildExternalTo(display, from, "", to, cc, subject, plain, html, parent, refs)
	return enqueueMail(owner, queuedMail{From: from, Subject: subject, MessageID: id,
		Message: signExternal(message), Recipients: append([]string{to}, cc...)})
}

func enqueueMail(owner string, m queuedMail) (string, error) {
	if owner == "" || m.From == "" || m.MessageID == "" || len(m.Message) == 0 {
		return "", fmt.Errorf("outgoing mail needs an owner, sender and message")
	}
	if len(m.Message) > maxOutgoingBytes*2 {
		return "", fmt.Errorf("outgoing message is too large")
	}
	seen := map[string]bool{}
	var recipients []string
	for _, addr := range m.Recipients {
		addr = strings.TrimSpace(addr)
		if addr == "" || strings.ContainsAny(addr, "\r\n") || !IsExternalAddress(addr) {
			return "", fmt.Errorf("invalid external recipient")
		}
		key := strings.ToLower(addr)
		if !seen[key] {
			recipients = append(recipients, addr)
			seen[key] = true
		}
	}
	if len(recipients) == 0 {
		return "", fmt.Errorf("no recipients")
	}
	m.Recipients, m.Delivered, m.Created = recipients, map[string]bool{}, time.Now().UTC()
	fields, err := outboxFields(m, true, time.Now())
	if err != nil {
		return "", err
	}
	if _, err := userdb.Create("mail", owner, outboxCollection, fields, false); err != nil {
		return "", err
	}
	select {
	case outboxWake <- struct{}{}:
	default:
	}
	return m.MessageID, nil
}

func outboxFields(m queuedMail, pending bool, next time.Time) (map[string]any, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	payload, err := encrypt(string(b))
	if err != nil {
		return nil, err
	}
	return map[string]any{"payload": payload, "pending": pending, "next": float64(next.Unix())}, nil
}

func readQueued(rec *userdb.Record) (queuedMail, error) {
	var m queuedMail
	payload, _ := rec.Data["payload"].(string)
	plain, err := decrypt(payload)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal([]byte(plain), &m)
	return m, err
}

func saveQueued(owner, id string, m queuedMail, pending bool, next time.Time) error {
	fields, err := outboxFields(m, pending, next)
	if err != nil {
		return err
	}
	_, err = userdb.Update("mail", owner, outboxCollection, id, fields, false)
	return err
}

// processQueued takes its transport as an argument so failures and restart
// recovery can be tested without an SMTP connection or a package-level hook.
func processQueued(owner, id string, relay func(string, string, []byte) error) error {
	outboxRunMu.Lock()
	defer outboxRunMu.Unlock()
	rec, err := userdb.Get("mail", owner, outboxCollection, id)
	if err != nil {
		return err
	}
	if rec.Data["pending"] != true {
		return nil
	}
	m, err := readQueued(rec)
	if err != nil {
		return err
	}
	if m.Delivered == nil {
		m.Delivered = map[string]bool{}
	}
	if time.Since(m.Created) >= 48*time.Hour {
		m.LastError = "Delivery stopped after 48 hours. Review the recipients before retrying."
		return saveQueued(owner, id, m, false, time.Now())
	}
	m.Attempts++
	delay := time.Minute * time.Duration(1<<min(m.Attempts-1, 6))
	next := time.Now().Add(delay)
	// Persist the attempt before entering a transport that may not return.
	if err := saveQueued(owner, id, m, true, next); err != nil {
		return err
	}
	m.LastError = ""
	for _, addr := range m.Recipients {
		if m.Delivered[addr] {
			continue
		}
		RecordOutbound(m.MessageID, addr)
		if err := relay(m.From, addr, m.Message); err != nil {
			m.LastError = err.Error()
		} else {
			m.Delivered[addr] = true
		}
		// A successful recipient must never be retried merely because a later
		// recipient failed. If this write fails, stop rather than sending more.
		if err := saveQueued(owner, id, m, true, next); err != nil {
			return err
		}
	}
	if len(m.Delivered) == len(m.Recipients) {
		return userdb.Delete("mail", owner, outboxCollection, id)
	}
	return nil
}

func retryOutbox() {
	for {
		for _, acc := range auth.AllAccounts() {
			if acc == nil {
				continue
			}
			// Every processed record leaves the due set, so this also drains past
			// the storage page limit without a failing first page starving others.
			for {
				batch, err := userdb.List("mail", acc.ID, outboxCollection, "mine", map[string]any{
					"pending": true, "next": map[string]any{"lte": float64(time.Now().Unix())},
				}, "next", "asc", userdb.MaxListLimit)
				if err != nil || len(batch) == 0 {
					break
				}
				failed := false
				for _, rec := range batch {
					if err := processQueued(acc.ID, rec.ID, RelayToExternal); err != nil {
						app.Log("mail", "outbox %s: %v", rec.ID, err)
						failed = true
					}
				}
				if failed {
					break
				} // storage errors must not create a busy retry loop
			}
		}
		select {
		case <-outboxWake:
		case <-time.After(time.Minute):
		}
	}
}

func deleteOutbox(owner string) {
	outboxRunMu.Lock()
	defer outboxRunMu.Unlock()
	if _, err := userdb.DeleteOwner("mail", owner); err != nil {
		app.Log("mail", "deleting outbox for %s: %v", owner, err)
	}
}
