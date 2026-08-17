// Package thread is the system of record: what was said, to whom, on which
// conversation.
//
// Everything in Mu that a person does is a message. An email, a Discord DM, a
// WhatsApp reply, a line typed on the web, an answer an agent sent back. The
// clients differ in protocol and in nothing else, so the record of what passed
// through them is one thing, and this is it.
//
// # Why this is not a service
//
// A service is something a caller may choose to use. A system of record is not
// a choice: it is written on every turn, from every client, whether or not
// anybody asks. Making it a service would put the core of the product behind a
// decision an agent takes — and an agent that forgot to call it would simply
// stop remembering.
//
// So the writing lives here, in the substrate, alongside the others that are
// nobody's choice: internal/quota decides what things cost, internal/x402
// prices a request, internal/auth says who you are. None of them is a service
// and none of them should be.
//
// Reading is a different question, and a service over this one is welcome —
// `threads_search` is a perfectly good tool, because searching your own past is
// something an agent decides to do. The test for whether that has been built in
// the right place: delete the service and nothing breaks. Clients still record,
// the agent still gets its history, the pages still render. You lose only the
// ability to go looking on purpose.
//
// # Messages, not events
//
// A message is what somebody said. An event is anything that happened, and a
// log that accepts anything has no schema and cannot be queried a year later.
// There are already two event-shaped things here — service/stream is the
// instance's public timeline and internal/usage is the counters — and a third
// would be the fourth message-shaped thing in a catalogue that is meant to be
// readable. If a second kind of event ever needs this treatment, a message
// becomes a kind of event and the store does not change.
//
// # This is not a workflow
//
// agent.Flow is a workflow record: the steps a task took, the tools it called,
// how long it ran. That is how an answer was produced. This is what was said.
// They were one struct for a while, which is how a workflow record ended up
// standing in as conversation history, and they have different lifetimes: a
// workflow record is debugging and can expire, a message is memory and should
// not.
package thread

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mu/internal/data"
)

// Role is who wrote a message. Two values and no more: a message is from the
// person or from the thing answering them.
const (
	RolePerson = "person"
	RoleAgent  = "agent"
)

// Thread is one conversation on one client.
//
// Identified by the client and that client's own key for it — a channel id, a
// phone number, the first message id of a mail chain. Opaque here: only the
// client knows what a conversation is on its own service, and this stores
// whatever it says without interpreting it.
type Thread struct {
	ID      string `json:"id"`
	Account string `json:"account"`
	Client  string `json:"client"`
	Key     string `json:"key"`
	Subject string `json:"subject,omitempty"`
	// Agent is who the conversation is with — one of the account's own, by id,
	// or empty for the default. A conversation is with somebody, and without
	// this a page that has an agent selected has to either show every
	// conversation on the account or none: both were tried and both read as the
	// surface having lost track of which agent you were talking to.
	Agent string `json:"agent,omitempty"`
	// Parties is everybody on this conversation: the account it belongs to, the
	// agent answering, and anybody else who has written in or been added.
	//
	// A conversation had exactly two sides — an owner and an agent — which is
	// true of a chat on a page and of nothing else. Mail already breaks it: a
	// stranger writes to agent@, the agent answers, and the owner is a third
	// party who can read it and was never in the exchange. A group is four
	// people and an agent. An agent writing to another agent is two agents and
	// nobody.
	//
	// This records who is on a conversation. It does not yet decide who may read
	// one — Get, Messages and Search are still scoped to Account, so being a
	// party to somebody else's thread grants nothing. Widening that is a
	// separate decision and a deliberate one; recording it first is what makes
	// the decision possible.
	Parties []Party   `json:"parties,omitempty"`
	Started time.Time `json:"started"`
	Updated time.Time `json:"updated"`
}

// Party is somebody on a conversation.
//
// Identified by a key rather than by an account, because most of them are not
// accounts here: a stranger who emailed in is an address, a Discord user is an
// id on somebody else's service. Key is whatever the client used to name them,
// and it is what a message's From matches.
type Party struct {
	// Kind is RolePerson or RoleAgent. The same two words a message uses,
	// because they answer the same question about a different noun.
	Kind string `json:"kind"`
	// Key is how this party is addressed on the client the conversation is on:
	// an email address, a phone number, a channel-scoped user id. Empty means
	// the account the conversation belongs to, which is how every message
	// written before this existed identifies its author.
	Key string `json:"key,omitempty"`
	// Name is what to call them, when the client knew. Falls back to Key.
	Name string `json:"name,omitempty"`
	// Agent is which agent, for a party of RoleAgent: one of the account's own
	// by id, or empty for the default.
	Agent  string    `json:"agent,omitempty"`
	Joined time.Time `json:"joined"`
}

// Message is one thing said.
type Message struct {
	ID      string    `json:"id"`
	Thread  string    `json:"thread"`
	Account string    `json:"account"`
	Role    string    `json:"role"`
	Text    string    `json:"text"`
	At      time.Time `json:"at"`
	// Ref is the client's own identifier for this message, where it has one —
	// a mail Message-ID. It is how a reply finds the conversation it continues
	// when the client knows better than the store does: answering something
	// from last week is answering *that*, not whatever has happened since.
	Ref string `json:"ref,omitempty"`
	// Workflow is the run that produced this message, when an agent wrote it.
	// A pointer rather than the steps themselves, because how an answer was
	// produced is a different question with a different lifetime.
	Workflow string `json:"workflow,omitempty"`
	// From is who wrote it, where that is not simply the account — somebody
	// else's address, on mail that was answered on the owner's behalf.
	From string `json:"from,omitempty"`
}

// maxPerAccount bounds one account's record.
//
// High, and deliberately much higher than the workflow store's 200: this is
// what an agent knows about somebody, and trimming it is forgetting. It exists
// because unbounded is not a plan, not because these are cheap to lose — when
// it starts binding, the answer is to distil old messages into memory before
// dropping them rather than to raise the number again.
const maxPerAccount = 5000

var (
	mu       sync.RWMutex
	threads  = map[string]*Thread{}    // id → thread
	messages = map[string][]*Message{} // thread id → messages, oldest first
)

func init() {
	var stored struct {
		Threads  []*Thread  `json:"threads"`
		Messages []*Message `json:"messages"`
	}
	if err := data.LoadJSON("threads.json", &stored); err != nil {
		return
	}
	for _, t := range stored.Threads {
		threads[t.ID] = t
	}
	for _, m := range stored.Messages {
		messages[m.Thread] = append(messages[m.Thread], m)
	}
	for id := range messages {
		sortByTime(messages[id])
	}
	go flusher()
}

// Open finds the conversation a client is on, or starts one.
//
// Scoped to the account as well as the client, because a thread key is a string
// somebody else's service chose — a phone number is not a secret, and two
// accounts may well be handed the same channel id.
func Open(account, client, key string) *Thread {
	if account == "" || client == "" || key == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()

	if t := findUnlocked(account, client, key); t != nil {
		return t
	}
	now := time.Now().UTC()
	t := &Thread{ID: newID(), Account: account, Client: client, Key: key,
		Started: now, Updated: now}
	threads[t.ID] = t
	save()
	return t
}

// Find returns an existing conversation, or nil.
func Find(account, client, key string) *Thread {
	mu.RLock()
	defer mu.RUnlock()
	return findUnlocked(account, client, key)
}

func findUnlocked(account, client, key string) *Thread {
	for _, t := range threads {
		if t.Account == account && t.Client == client && t.Key == key {
			return t
		}
	}
	return nil
}

// Get returns a conversation by id, scoped to its owner.
func Get(account, id string) *Thread {
	mu.RLock()
	defer mu.RUnlock()
	if t := threads[id]; t != nil && t.Account == account {
		return t
	}
	return nil
}

// ByRef finds the conversation containing a message the client named.
//
// Mail's use: a reply carries In-Reply-To and References, and any of those ids
// may be one we recorded. Scoped to the account, because a message id is a
// string somebody else's mail server wrote — unscoped, quoting one would attach
// a stranger to somebody else's conversation and hand over its history.
func ByRef(account string, refs ...string) *Thread {
	want := map[string]bool{}
	for _, r := range refs {
		for _, id := range Refs(r) {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return nil
	}

	mu.RLock()
	defer mu.RUnlock()

	// Newest first, so a long chain continues from its head rather than its
	// middle: a client quotes every id in the thread, and matching the oldest
	// would fan the conversation into a bush.
	var best *Message
	for _, msgs := range messages {
		for _, m := range msgs {
			if m.Account != account || m.Ref == "" || !want[m.Ref] {
				continue
			}
			if best == nil || m.At.After(best.At) {
				best = m
			}
		}
	}
	if best == nil {
		return nil
	}
	return threads[best.Thread]
}

// Refs pulls <...> bracketed ids out of a header value, which is the form mail
// uses. A value with no brackets is one id.
func Refs(s string) []string {
	var out []string
	rest := s
	for {
		start := strings.Index(rest, "<")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start:], ">")
		if end < 0 {
			break
		}
		out = append(out, rest[start:start+end+1])
		rest = rest[start+end+1:]
	}
	if len(out) == 0 {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Add records something said, and returns its id.
func Add(m Message) string {
	if m.Thread == "" || m.Account == "" || strings.TrimSpace(m.Text) == "" {
		return ""
	}
	mu.Lock()
	defer mu.Unlock()

	t := threads[m.Thread]
	if t == nil || t.Account != m.Account {
		return ""
	}
	if m.ID == "" {
		m.ID = newID()
	}
	if m.At.IsZero() {
		m.At = time.Now().UTC()
	}
	if m.Role == "" {
		m.Role = RolePerson
	}
	stored := m
	messages[m.Thread] = append(messages[m.Thread], &stored)
	t.Updated = stored.At
	if t.Subject == "" && stored.Role == RolePerson {
		t.Subject = summarise(stored.Text)
	}
	// Whoever spoke is on the conversation. Parties accrete from what actually
	// happened rather than needing a client to declare them, which means the
	// mail thread where a stranger wrote in has both of them on it without mail
	// knowing this exists.
	joinUnlocked(t, Party{Kind: stored.Role, Key: stored.From, Agent: t.Agent, Joined: stored.At})
	trim(m.Account)
	save()
	return stored.ID
}

// Join puts somebody on a conversation who has not written to it.
//
// Everything else here records what happened; this records who is expected to.
// A shared inbox is exactly the case that cannot be derived from messages — the
// colleague who has been added and has not said anything yet is still supposed
// to see it.
func Join(account, id string, p Party) {
	mu.Lock()
	defer mu.Unlock()
	t := threads[id]
	if t == nil || t.Account != account {
		return
	}
	if joinUnlocked(t, p) {
		save()
	}
}

// joinUnlocked adds a party if it is not already there. Caller holds mu.
//
// Matched on kind and key together: the owner writing and the agent answering
// are two parties with the same empty key, which is the common case and would
// otherwise collapse into one.
func joinUnlocked(t *Thread, p Party) bool {
	if p.Kind == "" {
		p.Kind = RolePerson
	}
	for i, have := range t.Parties {
		if have.Kind != p.Kind || have.Key != p.Key {
			continue
		}
		// Something learned late — a name the first message did not carry.
		if have.Name == "" && p.Name != "" {
			t.Parties[i].Name = p.Name
			return true
		}
		return false
	}
	if p.Joined.IsZero() {
		p.Joined = time.Now().UTC()
	}
	t.Parties = append(t.Parties, p)
	return true
}

// Parties is who is on a conversation, scoped to its owner.
func Parties(account, id string) []Party {
	mu.RLock()
	defer mu.RUnlock()
	t := threads[id]
	if t == nil || t.Account != account {
		return nil
	}
	return append([]Party(nil), t.Parties...)
}

// Messages returns a conversation's messages, oldest first. limit 0 is all of
// them; a positive limit returns the most recent that many, still in order.
func Messages(account, threadID string, limit int) []Message {
	mu.RLock()
	defer mu.RUnlock()

	t := threads[threadID]
	if t == nil || t.Account != account {
		return nil
	}
	src := messages[threadID]
	if limit > 0 && len(src) > limit {
		src = src[len(src)-limit:]
	}
	out := make([]Message, 0, len(src))
	for _, m := range src {
		out = append(out, *m)
	}
	return out
}

// List returns an account's conversations, most recently active first.
func List(account string, limit int) []Thread {
	mu.RLock()
	defer mu.RUnlock()

	var out []Thread
	for _, t := range threads {
		if t.Account == account {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// trim keeps an account's record within bounds, oldest first. Caller holds mu.
//
// Whole conversations rather than the oldest messages across all of them: half
// a conversation is worse than none, because what survives reads as the whole
// of it.
func trim(account string) {
	var total int
	var owned []*Thread
	for _, t := range threads {
		if t.Account != account {
			continue
		}
		owned = append(owned, t)
		total += len(messages[t.ID])
	}
	if total <= maxPerAccount {
		return
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Updated.Before(owned[j].Updated) })
	for _, t := range owned {
		if total <= maxPerAccount {
			return
		}
		total -= len(messages[t.ID])
		delete(messages, t.ID)
		delete(threads, t.ID)
	}
}

// save marks the store dirty. Caller holds mu.
//
// It used to marshal every message on this instance and fsync the file, while
// holding the write lock, on every single message — so the whole record was
// rewritten twice per turn of every conversation, and every reader waited
// behind it. Adoption made that visible: it writes a message at a time, so
// carrying a few hundred old conversations across meant a few hundred full-file
// writes with the lock held, and the UI stopped answering.
//
// Now a write is a flag. The flusher below does the work, at most once a second,
// with the lock released while it marshals and writes. The cost is that a crash
// can lose the last second of conversation; the alternative was freezing the
// product to avoid it.
func save() { dirty.Store(true) }

var (
	dirty   atomic.Bool
	writeMu sync.Mutex // one writer at a time, held only around the write
)

// flushInterval bounds how stale the file on disk can be.
const flushInterval = time.Second

func flusher() {
	for range time.Tick(flushInterval) {
		Flush()
	}
}

// Flush writes the store out now, if anything changed.
//
// Exported for the two callers that cannot wait for the tick: a test that wants
// to know the file is on disk, and anything shutting down.
func Flush() {
	if !dirty.Swap(false) {
		return
	}
	var stored struct {
		Threads  []*Thread  `json:"threads"`
		Messages []*Message `json:"messages"`
	}
	// A copy under the read lock, so marshalling and the fsync happen with
	// nothing blocked behind them. The pointers are shared with the live maps,
	// which is why the copy is taken while readers are excluded from writing:
	// every mutation happens under the write lock, so nothing is being changed
	// underneath the marshal.
	mu.RLock()
	for _, t := range threads {
		stored.Threads = append(stored.Threads, t)
	}
	for _, msgs := range messages {
		stored.Messages = append(stored.Messages, msgs...)
	}
	mu.RUnlock()

	writeMu.Lock()
	defer writeMu.Unlock()
	data.SaveJSON("threads.json", stored) //nolint:errcheck
}

func sortByTime(m []*Message) {
	sort.Slice(m, func(i, j int) bool { return m[i].At.Before(m[j].At) })
}

// summarise makes a one-line name for a conversation from its first message.
func summarise(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	if len(text) > 80 {
		text = strings.TrimSpace(text[:80]) + "…"
	}
	return text
}

func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// SetAgent records who a conversation is with.
//
// Set on every turn rather than at Open, because Open is also how an existing
// conversation is found and the answer can change: writing to agent+news@ after
// a week of writing to agent@ is a conversation with news from that point on.
// The last agent to answer is the one the conversation is with.
func SetAgent(account, id, agentID string) {
	mu.Lock()
	defer mu.Unlock()
	t := threads[id]
	if t == nil || t.Account != account || t.Agent == agentID {
		return
	}
	t.Agent = agentID
	save()
}

// Delete removes a conversation and everything said on it.
//
// The record is memory and trimming it is forgetting, which is why nothing
// expires it — but it is somebody's own memory, and being unable to delete a
// conversation from your own account is not a durability guarantee, it is a
// missing button.
//
// Silent about whether there was one: every caller is a person clicking Delete,
// and "there was nothing there" and "it is gone now" are the same outcome to
// them.
func Delete(account, id string) {
	mu.Lock()
	defer mu.Unlock()
	if t := threads[id]; t == nil || t.Account != account {
		return
	}
	delete(threads, id)
	delete(messages, id)
	save()
}

// Forget removes an account's whole record: every conversation, everything said
// on any of them.
//
// For account deletion, which had no way to reach this store — the record is
// written by the machinery rather than by a service, so nothing owned it and
// nothing cleared it. Deleting your account left the transcript of every
// conversation you had ever had on disk, which is the worst possible thing to
// forget to delete.
func Forget(account string) {
	if account == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for id, t := range threads {
		if t.Account != account {
			continue
		}
		delete(threads, id)
		delete(messages, id)
	}
	save()
}

// SetRef records the client's identifier for a message after the fact.
//
// An outbound id does not exist until the message has actually been sent, and
// the message is written down before that — a reply that fails to send still
// has to be in the record. Without this the answer to an answer would not find
// the conversation it belongs to.
func SetRef(account, messageID, ref string) {
	if ref = strings.TrimSpace(ref); ref == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, msgs := range messages {
		for _, m := range msgs {
			if m.ID == messageID && m.Account == account {
				m.Ref = ref
				save()
				return
			}
		}
	}
}
