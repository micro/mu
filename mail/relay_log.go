// Outbound relay log.
//
// Every message Mu hands to an external MX goes through RelayToExternal —
// user mail composed at /mail, but also system mail like calendar invites for
// events and account verification, which are never written to the message
// store and so were previously invisible. This records each attempt (success
// and failure) at that single choke point, so the admin mail log reflects
// everything that actually left the server.
package mail

import (
	"bytes"
	"fmt"
	netmail "net/mail"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// RelayEntry is one attempt to deliver a message to an external MX.
type RelayEntry struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	MessageID string    `json:"message_id"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
}

// relayLogMax caps the log. Old entries fall off the end.
const relayLogMax = 500

var (
	relayMu  sync.RWMutex
	relayLog []*RelayEntry // newest first
)

func init() {
	data.LoadJSON("mail_relay_log.json", &relayLog) //nolint:errcheck
}

// recordRelay appends a relay attempt to the log. data is the raw RFC 5322
// message we handed over; Subject and Message-ID are read back out of it so
// every caller is covered without having to thread metadata through.
func recordRelay(from, to string, msg []byte, relayErr error) {
	e := &RelayEntry{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Time:    time.Now(),
		From:    from,
		To:      to,
		Subject: "(no subject)",
		OK:      relayErr == nil,
	}
	if relayErr != nil {
		e.Error = relayErr.Error()
	}
	if subject, messageID, ok := relayHeaders(msg); ok {
		if subject != "" {
			e.Subject = subject
		}
		e.MessageID = messageID
	}

	relayMu.Lock()
	relayLog = append([]*RelayEntry{e}, relayLog...)
	if len(relayLog) > relayLogMax {
		relayLog = relayLog[:relayLogMax]
	}
	snapshot := make([]*RelayEntry, len(relayLog))
	copy(snapshot, relayLog)
	relayMu.Unlock()

	data.SaveJSON("mail_relay_log.json", snapshot) //nolint:errcheck
}

// relayHeaders pulls Subject and Message-ID off a raw message.
func relayHeaders(msg []byte) (subject, messageID string, ok bool) {
	parsed, err := netmail.ReadMessage(bytes.NewReader(msg))
	if err != nil {
		return "", "", false
	}
	return decodeMIMEHeader(parsed.Header.Get("Subject")),
		strings.TrimSpace(parsed.Header.Get("Message-ID")), true
}

// RecentRelays returns the most recent outbound relay attempts, newest first.
func RecentRelays(limit int) []RelayEntry {
	relayMu.RLock()
	defer relayMu.RUnlock()

	if limit <= 0 || limit > len(relayLog) {
		limit = len(relayLog)
	}
	out := make([]RelayEntry, 0, limit)
	for _, e := range relayLog[:limit] {
		out = append(out, *e)
	}
	return out
}

// RelayStats returns the number of logged relay attempts that succeeded and
// failed. Bounded by relayLogMax, so it is a recent-activity figure rather
// than an all-time total.
func RelayStats() (sent, failed int) {
	relayMu.RLock()
	defer relayMu.RUnlock()

	for _, e := range relayLog {
		if e.OK {
			sent++
		} else {
			failed++
		}
	}
	return
}
