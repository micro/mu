package mail

import (
	"fmt"
	"testing"
	"time"
)

// What one delivery costs as a mailbox fills up.
//
// Filing a message does two things over the *whole* store, not over the
// message: rebuildInboxes reconstructs every account's threads from every
// message, and save marshals, encrypts and rewrites the entire mail.json. Both
// run under the write mutex, so every other reader and writer waits.
//
// That makes delivery O(total messages) rather than O(1), which is the shape
// that is fine in testing and gets slowly worse forever in production. This
// measures it so the issue carries a number instead of a claim.
//
//	go test ./service/mail/ -run xxx -bench Delivery -benchtime 20x
func BenchmarkDeliveryAsTheStoreGrows(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("%d-messages-already-stored", size), func(b *testing.B) {
			b.Setenv("MAIL_DOMAIN", "example.test")
			seedStore(b, size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := SendMessageTo(Delivery{
					From: "Someone", FromID: "someone@elsewhere.test",
					To: "benchowner", ToID: "benchowner",
					Subject:   "benchmark",
					Body:      "a message of an ordinary size, which is what most mail is",
					MessageID: fmt.Sprintf("<bench.%d.%d@example.test>", size, i),
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// And the read the agent does every time somebody asks it to check their mail.
func BenchmarkListMessagesAsTheStoreGrows(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("%d-messages-already-stored", size), func(b *testing.B) {
			b.Setenv("MAIL_DOMAIN", "example.test")
			seedStore(b, size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := ListMessages("benchowner", 10); len(got) == 0 {
					b.Fatal("nothing listed")
				}
			}
		})
	}
}

// seedStore fills the store directly, without going through the delivery path
// — the point is to measure one delivery into a store of a given size, not to
// pay for building it.
func seedStore(tb testing.TB, n int) {
	tb.Helper()
	mutex.Lock()
	saved := messages
	built := make([]*Message, 0, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%d", now.Add(-time.Duration(i)*time.Minute).UnixNano())
		built = append(built, &Message{
			ID: id, ThreadID: id,
			From: "Someone", FromID: "someone@elsewhere.test",
			To: "benchowner", ToID: "benchowner",
			Subject:   "seeded",
			Body:      "a message of an ordinary size, which is what most mail is",
			MessageID: fmt.Sprintf("<seed.%d@example.test>", i),
			CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	messages = built
	rebuildInboxes()
	mutex.Unlock()

	tb.Cleanup(func() {
		mutex.Lock()
		messages = saved
		rebuildInboxes()
		mutex.Unlock()
	})
}
