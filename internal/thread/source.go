package thread

import "strings"

// Source points to a service-owned message. Text in a projected message is a
// materialized content, refreshed by Inbox on events. Pages never fetch sources.
type Source struct {
	Service string `json:"service"`
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// InvalidateSource clears cached content after the owning service deletes it.
func InvalidateSource(account, serviceName, id string) {
	ensure()
	mu.Lock()
	defer mu.Unlock()
	for threadID := range owned[account] {
		for _, m := range messages[threadID] {
			if m.Source != nil && m.Source.Service == serviceName && m.Source.ID == id {
				m.Text = "(message deleted)"
				m.SourceHTML = ""
				save()
			}
		}
	}
}

// LatestTo finds the newest cached incoming message addressed to a local
// mailbox. It reads the existing projection without sorting or copying an
// account's history. Source invalidation removes deleted content from this view.
func LatestTo(account, localPart string) *Message {
	ensure()
	mu.RLock()
	defer mu.RUnlock()
	var latest *Message
	for id, t := range owned[account] {
		if t.Held {
			continue
		}
		for _, m := range messages[id] {
			to, _, _ := strings.Cut(m.To, "@")
			if to != localPart || m.Text == "(message deleted)" {
				continue
			}
			if latest == nil || m.At.After(latest.At) {
				latest = m
			}
		}
	}
	if latest == nil {
		return nil
	}
	out := *latest
	return &out
}
