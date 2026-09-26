package chat

// Chat owns protocol and room records. Committed arrival events identify
// those records; Inbox and Agent read them through Server.Source independently.
// Recording a message does not depend on an agent choosing to answer.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/event"
)

// newID is a message's own identifier, which is what a client pages from.
//
// Random rather than a counter, because MAM hands these to a client as opaque
// cursors and a guessable one invites asking for somebody else's — the archive
// is scoped to the account either way, but a cursor is a thing a client keeps
// and repeats back, so it should carry no meaning.
func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// Said is one message on one conversation.
//
// Addressed rather than authored: From and To are JIDs, because that is what a
// client sent and what MAM has to hand back. The record above stores an author
// and a role, which is the right shape for memory and the wrong one for a
// protocol that addresses everything.
type Said struct {
	OMEMO  string                 `json:"omemo,omitempty"`
	WireID string                 `json:"wire_id,omitempty"`
	Facts  map[string]interface{} `json:"facts,omitempty"`
	ID     string                 `json:"id"`
	Conv   string                 `json:"conv"` // the conversation key — see xmppRoom
	From   string                 `json:"from"`
	To     string                 `json:"to"`
	Text   string                 `json:"text"`
	At     time.Time              `json:"at"`
}

// heldPerAccount bounds one account's chat history.
//
// Lower than the record's, deliberately: this is a transcript, and the thing
// worth keeping for a year is the prose copy an agent remembers rather than
// every stanza that carried it.
const heldPerAccount = 2000

var (
	saidMu sync.RWMutex
	// said is every message, by the account whose record it is. Both sides of
	// a conversation between two accounts here are stored twice, once each,
	// for the reason mail stores a copy per mailbox: an account's history is
	// its own, and deleting yours must not reach into somebody else's.
	said = map[string][]*Said{}
)

// LoadStore reads the chat record from disk.
func LoadStore() {
	b, err := data.LoadFile("chat.json")
	if err != nil || len(b) == 0 {
		return
	}
	saidMu.Lock()
	defer saidMu.Unlock()
	if err := json.Unmarshal(b, &said); err != nil {
		app.Log("chat", "could not read the chat record: %v", err)
	}
}

// Keep writes a message to the archive. Callers needing an acknowledgement use
// KeepSaved so a failed disk write cannot be mistaken for durable delivery.
func Keep(account string, m Said) string {
	id, err := KeepSaved(account, m)
	if err != nil {
		app.Log("chat", "could not save message: %v", err)
	}
	return id
}

// KeepSaved saves before publishing the new archive state. Serializing the
// complete write with mutations also prevents an old snapshot overwriting a
// newer one when two messages arrive together.
func KeepSaved(account string, m Said) (string, error) {
	return keepSaved(account, m, nil, "")
}

func keepSaved(account string, m Said, writes map[string][]byte, topic string) (string, error) {
	if account == "" || strings.TrimSpace(m.Text) == "" || m.Conv == "" {
		return "", fmt.Errorf("a chat message needs an account, conversation and text")
	}
	if m.ID == "" {
		m.ID = newID()
	}
	if m.At.IsZero() {
		m.At = time.Now().UTC()
	}
	saidMu.Lock()
	defer saidMu.Unlock()
	previous := said[account]
	next := append(append([]*Said(nil), previous...), &m)
	if len(next) > heldPerAccount {
		next = next[len(next)-heldPerAccount:]
	}
	said[account] = next
	facts := []event.Record{{Type: event.ChatRecorded, Service: "chat", Account: account, Resource: m.ID, Version: m.At.UTC().Format(time.RFC3339Nano)}}
	if topic != "" {
		facts = append(facts, event.Record{Type: topic, Service: "chat", Account: account, Resource: m.ID})
	}
	if writes == nil {
		writes = map[string][]byte{}
	}
	b, err := json.Marshal(said)
	if err == nil {
		writes["chat.json"] = b
		err = event.Commit(writes, facts...)
	}
	if err != nil {
		said[account] = previous
		return "", err
	}
	return m.ID, nil
}

// Conversation is what was said on one conversation, oldest first.
func Conversation(account, conv string, limit int) []Said {
	return filtered(account, limit, func(m *Said) bool { return m.Conv == conv })
}

// Everything is every conversation this account has had here, oldest first.
func Everything(account string, limit int) []Said {
	return filtered(account, limit, func(*Said) bool { return true })
}

func filtered(account string, limit int, keep func(*Said) bool) []Said {
	if account == "" {
		return nil
	}
	saidMu.RLock()
	defer saidMu.RUnlock()

	var out []Said
	for _, m := range said[account] {
		if keep(m) {
			out = append(out, *m)
		}
	}
	// Oldest first, which is the order MAM requires and the order a client
	// renders. Sorted rather than assumed: two carriers append concurrently.
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Forget drops an account's chat record.
//
// Called when an account is deleted. A service that keeps something about a
// person and cannot be told to stop is the thing every deletion hook exists to
// prevent — see TestEveryScopedServiceCleansUpWhenAnAccountIsDeleted.
func Forget(account string) {
	forgetOMEMO(account)
	forgetPrivate(account)
	saidMu.Lock()
	defer saidMu.Unlock()
	delete(said, account)
	if err := data.SaveJSON("chat.json", said); err != nil {
		app.Log("chat", "could not delete chat archive: %v", err)
	}
}
