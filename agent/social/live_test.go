package social

import (
	"os"
	"testing"
	"time"
)

// TestAgainstTheRealFirehose is a probe, not a test: it runs only when somebody
// asks for it, because it needs the network and it needs a minute.
//
//	SOCIAL_LIVE=1 go test ./agent/social/ -run TestAgainstTheRealFirehose -v
//
// It exists because the filters are guesses about a stream nobody can see from
// a unit test — how much survives, whether the categories match anything real,
// and whether what comes out is worth reading. That is answerable by looking.
func TestAgainstTheRealFirehose(t *testing.T) {
	if os.Getenv("SOCIAL_LIVE") == "" {
		t.Skip("set SOCIAL_LIVE=1 to watch the real network")
	}
	t.Setenv("SOCIAL_ATPROTO", "true")

	seen, kept := 0, 0
	Surface = func(c *candidate) {
		kept++
		t.Logf("[%s] score %d  %s\n    %s", c.Category, c.Score, c.host(), c.display())
	}
	watched = func(ev event) {
		if ev.Commit.Operation == "create" {
			seen++
		}
	}
	defer func() { Surface = func(*candidate) {}; watched = nil }()

	// stream rather than Watch: Watch also starts the review loop, which waits
	// two minutes before its first pass. A probe that stops before then reports
	// nothing surfaced and looks like a broken selector, which is how the first
	// run of this read.
	go stream() //nolint:errcheck
	time.Sleep(60 * time.Second)

	mu.Lock()
	passed := len(candidates)
	mu.Unlock()

	// Drive one review by hand — the point of the probe is to see what a reader
	// would actually be given, not how full the buffer got.
	review()

	t.Logf("saw %d posts, %d passed the filters, %d surfaced", seen, passed, kept)
	if seen == 0 {
		t.Fatal("no events at all — the firehose is unreachable from here")
	}
	if passed > 0 && kept == 0 {
		t.Errorf("%d candidates and a review surfaced none of them", passed)
	}
}
