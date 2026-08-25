package chat

// What was said here, kept here.
//
// # Why this exists
//
// service/mail owns mail: envelopes, MIME, folders, spam flags, Message-IDs.
// It does not import internal/thread and never has. The prose copy an agent
// remembers is written by agent/mail, above it, and joined back by the
// Message-ID — so the service is self-contained and the thing built on top is
// the thing that reaches into the record.
//
// Chat had that backwards for a day. xmpp_record.go wrote stanzas straight into
// internal/thread, which no other protocol service does, and it meant two
// things that were both wrong: a person-to-person message with no agent in it
// turned up in an inbox nobody addressed, and the archive a client asks for was
// a prose rendering rather than what was actually said.
//
// So: this is chat's record. Stanzas, with the ids and timestamps a client
// gets back from MAM. agent/chat is what writes a prose copy into the record
// for the agent to remember, which is the same seam agent/mail is.
//
// # What follows from being self-contained
//
// A conversation between two people never reaches internal/thread, because no
// agent was in it and there is nothing for one to remember. That is not a
// filter applied afterwards — it is what happens when the service keeps its own
// record. Which is also the answer to why chat stopped appearing in /inbox.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
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
	ID   string    `json:"id"`
	Conv string    `json:"conv"` // the conversation key — see xmppRoom
	From string    `json:"from"`
	To   string    `json:"to"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
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

func saveStore() {
	b, err := json.Marshal(said)
	if err != nil {
		app.Log("chat", "could not marshal the chat record: %v", err)
		return
	}
	if err := data.SaveFile("chat.json", string(b)); err != nil {
		app.Log("chat", "could not write the chat record: %v", err)
	}
}

// Keep writes one message down, for one account.
//
// Returns the id it was stored under, which is what MAM hands a client as the
// archive id and what a client pages backwards from.
func Keep(account string, m Said) string {
	if account == "" || strings.TrimSpace(m.Text) == "" || m.Conv == "" {
		return ""
	}
	if m.ID == "" {
		m.ID = newID()
	}
	if m.At.IsZero() {
		m.At = time.Now().UTC()
	}

	saidMu.Lock()
	said[account] = append(said[account], &m)
	if n := len(said[account]); n > heldPerAccount {
		said[account] = said[account][n-heldPerAccount:]
	}
	saidMu.Unlock()

	saveStore()
	return m.ID
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
	saidMu.Lock()
	delete(said, account)
	saidMu.Unlock()
	saveStore()
}
