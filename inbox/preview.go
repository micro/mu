package inbox

import "mu/internal/thread"

// Waiting counts unread conversations in the same local selection as Inbox.
// Unread is read state, not a claim that a reply or action is needed.
func Waiting(accountID string) (unread int, newest string) {
	for _, t := range inboxThreads(accountID, "/inbox") {
		if !thread.Unread(t) {
			continue
		}
		if unread == 0 {
			// arrivals is newest first, so the first unread one is the newest.
			newest, _ = party(accountID, t)
		}
		unread++
	}
	return unread, newest
}
