package thread

// Going looking, on purpose.
//
// Everything else here is written and read by the machinery: a client records a
// turn, the agent asks for the last six messages, a page renders a
// conversation. None of that is a search. This is the read that somebody
// decides to do — "what did I tell it about the invoice" — and it is the one
// thing over this store that is properly a service. See service/recall.
//
// A scan rather than an index. An account holds at most maxPerAccount messages,
// the store is already in memory, and the index that exists (internal/data's
// FTS5 table) is for public content — putting private conversations in it is a
// bigger decision than making this fast. When a scan stops being fast enough,
// that decision has been earned.

import (
	"sort"
	"strings"
)

// Hit is a message that matched, with the conversation it was said on.
type Hit struct {
	Message
	Client  string `json:"client"`
	Subject string `json:"subject,omitempty"`
}

// Search finds messages in an account's record, most recent first.
//
// Case-insensitive substring, because the alternative is a query language
// nobody asked for and every caller here is either a person typing a phrase or
// an agent passing one on. An empty query matches nothing rather than
// everything: "search my history for ”" is a mistake, and answering it with
// the whole record is the wrong way to fail.
//
// client narrows to one — "web", "mail" — and is ignored when empty.
func Search(account, query, client string, limit int) []Hit {
	query = strings.TrimSpace(query)
	if account == "" || query == "" {
		return nil
	}
	want := strings.ToLower(query)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	mu.RLock()
	defer mu.RUnlock()

	var hits []Hit
	for id, t := range owned[account] {
		if client != "" && t.Client != client {
			continue
		}
		for _, m := range messages[id] {
			if !strings.Contains(strings.ToLower(m.Text), want) {
				continue
			}
			hits = append(hits, Hit{Message: *m, Client: t.Client, Subject: t.Subject})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].At.After(hits[j].At) })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
