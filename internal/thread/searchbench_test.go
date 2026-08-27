package thread

// What the record costs to search, at the size it is allowed to reach.
//
// Written to answer "should this be in SQLite" with a number rather than an
// instinct. service/mail asked the same question and measured 71ms over 5,000
// messages before it moved to the FTS5 index — see service/mail/index.go, which
// is the worked example for doing this.
//
// The answer here is different, and the number is why: a search over a full
// account is under a millisecond, because the scan is bounded. maxPerAccount
// caps an account at 5,000 messages and trim drops whole conversations past it.
// So the scan is not the thing to fix. The cap is.

import (
	"fmt"
	"testing"
)

func seed(b *testing.B, account string, threads, per int) {
	b.Helper()
	for i := 0; i < threads; i++ {
		t := Open(account, "sms", fmt.Sprintf("+4477009%05d", i))
		if t == nil {
			b.Fatal("no thread")
		}
		for j := 0; j < per; j++ {
			// One conversation in fifty mentions the word, which is what a
			// real query looks like. Seeding it into every message measures
			// allocating five thousand hits rather than searching for them.
			text := fmt.Sprintf("message %d of conversation %d, ordinary chatter about nothing", j, i)
			if i%50 == 0 && j == 0 {
				text = "here is the invoice you asked for"
			}
			Add(Message{Thread: t.ID, Account: account, Text: text})
		}
	}
}

func BenchmarkSearchSmall(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	seed(b, "small", 50, 10) // 500 messages
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Search("small", "invoice", "", 50)
	}
}

func BenchmarkSearchFull(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	seed(b, "full", 500, 10) // 5,000 messages — maxPerAccount
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Search("full", "invoice", "", 50)
	}
}

func BenchmarkSearchMiss(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	seed(b, "miss", 500, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Search("miss", "zzzznothingmatches", "", 50)
	}
}
