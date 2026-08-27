package chat

import (
	"testing"

	"mu/internal/thread"
)

// recorded is how many messages this account has in the system of record.
//
// In its own file with the one import, so TestChatDoesNotWriteToTheRecord scans
// the package's source and finds nothing — a test may look at the record to
// prove the service does not touch it, which is the opposite of the thing being
// forbidden.
func recorded(t *testing.T, account string) int {
	t.Helper()
	n := 0
	for _, th := range thread.List(account, 100) {
		n += len(thread.Messages(account, th.ID, 100))
	}
	return n
}
