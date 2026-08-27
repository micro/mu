package push

// What has been sent, and what became of it.
//
// The store knew two things about a notification: that a device exists, and the
// outcome of the most recent attempt to reach it. Neither answers the question
// somebody actually has, which is "what have you been telling me" — and neither
// answers the operator's version, "did the thing I wired up ever fire". A
// notification is the one message this product sends that leaves no copy
// anywhere: mail is in the mailbox, a text is in the record, an answer is in the
// thread, and a push disappeared into the handset.
//
// So it is written down. Not for delivery — that is Subscription.Sent — but for
// reading: a short list, newest last, capped, per account.

import (
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// Sent is one notification that was sent, and what happened to it.
type Sent struct {
	Title string    `json:"title"`
	Body  string    `json:"body,omitempty"`
	URL   string    `json:"url,omitempty"`
	At    time.Time `json:"at"`
	// From is what asked for it — "mail", "reminder", an agent's name. A list of
	// notifications with no sender is a list of interruptions with nobody to
	// hold responsible for them, which is how a person ends up turning the whole
	// feature off rather than the one thing that is being noisy.
	From string `json:"from,omitempty"`
	// OK is whether any device took it. Error is why not, when not.
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Got is when a device confirmed it woke up holding this.
	//
	// "The push service accepted it" is where the record used to end, and it is
	// three quarters of the way to an answer. A notification that FCM takes and
	// the handset never shows looks exactly like one that was never sent, and no
	// amount of care on this side can tell them apart — the server has no way to
	// see a service worker. So the service worker says so: it posts a receipt
	// when the push event fires. Sent, accepted, arrived, shown — four steps,
	// and until this existed the last two were one dark room.
	Got time.Time `json:"got,omitempty"`
	// Shown is false when the device woke up and could not render it, with Why
	// saying what it could not do. A receipt that only ever means good news is
	// not a receipt.
	Shown bool   `json:"shown,omitempty"`
	Why   string `json:"why,omitempty"`
	// Tag is what the receipt is matched on.
	Tag string `json:"tag,omitempty"`
}

// keep is how many notifications are remembered per account.
//
// Short on purpose. This is a receipt, not an archive: the question is "what
// arrived recently and did it work", and a thousand rows answer that no better
// than fifty while making the file something that has to be thought about.
const keep = 50

const historyFile = "push_history.json"

var (
	histMu   sync.RWMutex
	hist     = map[string][]Sent{}
	histOnce sync.Once
)

func loadHistory() {
	histOnce.Do(func() {
		data.LoadJSON(historyFile, &hist) //nolint:errcheck
		if hist == nil {
			hist = map[string][]Sent{}
		}
	})
}

// note records one notification against an account.
//
// Synchronous, like save, and for the same reason: a goroutine here means the
// write lands after the caller has moved on, which in a test is a file arriving
// during cleanup and in production is a process that exits between the send and
// the write, losing the only copy that existed.
func note(account string, s Sent) {
	if account == "" {
		return
	}
	if s.At.IsZero() {
		s.At = time.Now().UTC()
	}
	loadHistory()
	histMu.Lock()
	defer histMu.Unlock()
	list := append(hist[account], s)
	if len(list) > keep {
		list = list[len(list)-keep:]
	}
	hist[account] = list
	snapshot := make(map[string][]Sent, len(hist))
	for k, v := range hist {
		snapshot[k] = append([]Sent(nil), v...)
	}
	data.SaveJSON(historyFile, snapshot) //nolint:errcheck
}

// Received is a device saying it woke up holding a notification.
//
// Matched on the tag against the most recent notification carrying it, because
// that is the only identifier both ends have: the server chose it and the
// service worker reads it back out of the payload it decrypted. Matching on
// anything else would mean minting an id and threading it through, for a
// receipt that only ever concerns the newest one.
//
// Unknown tags are dropped rather than appended. This is reachable by anything
// holding a session, so an unmatched tag must not be able to grow the file.
func Received(account, tag string, shown bool, why string) {
	if account == "" {
		return
	}
	tag = strings.TrimSpace(tag)
	if len(tag) > 120 {
		tag = tag[:120]
	}
	if len(why) > 200 {
		why = why[:200]
	}
	loadHistory()
	histMu.Lock()
	defer histMu.Unlock()
	list := hist[account]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].Tag != tag || !list[i].Got.IsZero() {
			continue
		}
		list[i].Got = time.Now().UTC()
		list[i].Shown = shown
		list[i].Why = why
		hist[account] = list
		snapshot := make(map[string][]Sent, len(hist))
		for k, v := range hist {
			snapshot[k] = append([]Sent(nil), v...)
		}
		data.SaveJSON(historyFile, snapshot) //nolint:errcheck
		return
	}
}

// History is what has been sent to an account, newest first.
//
// Newest first because every list in this product is, and because the question
// is almost always about the last one.
func History(account string, limit int) []Sent {
	loadHistory()
	histMu.RLock()
	list := append([]Sent(nil), hist[account]...)
	histMu.RUnlock()

	sort.Slice(list, func(i, j int) bool { return list[i].At.After(list[j].At) })
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

// ForgetHistory drops an account's notifications, for account deletion.
//
// Forget already removed the devices and left this behind, which is a record of
// what somebody was told sitting in a file after the account that was told it is
// gone.
func ForgetHistory(account string) {
	loadHistory()
	histMu.Lock()
	defer histMu.Unlock()
	delete(hist, account)
	snapshot := make(map[string][]Sent, len(hist))
	for k, v := range hist {
		snapshot[k] = append([]Sent(nil), v...)
	}
	data.SaveJSON(historyFile, snapshot) //nolint:errcheck
}

// source is who asked, cleaned up for the record. Empty when nobody said.
func source(from string) string {
	from = strings.TrimSpace(from)
	if len(from) > 60 {
		from = from[:60]
	}
	return from
}
