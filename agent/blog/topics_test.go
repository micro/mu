package blog

// A piece per topic means there has to be more than one topic.
//
// service/blog/topics.json is embedded, so what it contains is what the blog
// is about, for the life of the binary. It contained ["Islam"] — added in a
// commit about something else entirely — and the eight-name list in
// service/blog.Load is only the fallback for an unmarshal error, so it never
// ran. The blog said it was about one subject and the opinion agent had one
// category to cycle through, which with one piece a day meant it published the
// same category every day.
//
// Nothing failed. That is why this is a test: a topic list that is quietly one
// entry long produces a working instance that writes about one thing.

import (
	"os"
	"testing"
	"time"
)

func TestThereIsMoreThanOneTopicToWriteAbout(t *testing.T) {
	cats := opinionCategories()
	if len(cats) < 2 {
		t.Fatalf("the blog has %d topics (%v) — an opinion piece per topic is one "+
			"piece, on the same subject, every day", len(cats), cats)
	}
	seen := map[string]bool{}
	for _, c := range cats {
		if c == "" {
			t.Error("a topic is the empty string, which tags a post with nothing")
		}
		if seen[c] {
			t.Errorf("%q is in the topic list twice, so one day's slot is spent twice", c)
		}
		seen[c] = true
	}
}

// One piece per topic per day, up to the ceiling.
func TestEveryTopicGetsAPieceInADay(t *testing.T) {
	n := len(opinionCategories())
	want := n
	if want > maxDailyOpinions {
		want = maxDailyOpinions
	}
	if got := dailyOpinions(); got != want {
		t.Errorf("dailyOpinions() = %d for %d topics, want %d — a topic that gets a "+
			"piece every N days is not covered, it is visited", got, n, want)
	}

	// And they fit in a day. opinionInterval spreads them across the sixteen
	// waking hours but clamps at an hour, so a long enough topic list would
	// schedule more pieces than there are slots and the last few would never
	// be written.
	if span := time.Duration(dailyOpinions()) * opinionInterval(dailyOpinions()); span > 24*time.Hour {
		t.Errorf("%d pieces at %v apart is %v of publishing — the ones at the end "+
			"of the list never get written", dailyOpinions(), opinionInterval(dailyOpinions()), span)
	}
}

// And an instance that does not want them can say so.
//
// The ceiling bounds the cost; this removes it. Each piece is a catalogue
// gather, a web research pass and a generation, billed to Micro's own account,
// and somebody self-hosting against a paid model is entitled to a blog that
// does not write itself. The alternative — editing topics.json down to nothing
// — reads as a mistake rather than a decision.
func TestOpinionsCanBeTurnedOff(t *testing.T) {
	for _, off := range []string{"off", "OFF", "false", "no", "0", "none"} {
		t.Setenv("OPINIONS", off)
		if opinionsEnabled() {
			t.Errorf("OPINIONS=%q still writes", off)
		}
	}
	for _, on := range []string{"", "on", "true", "yes"} {
		t.Setenv("OPINIONS", on)
		if !opinionsEnabled() {
			t.Errorf("OPINIONS=%q does not write — the default has to be on, or an "+
				"upgrade silently stops the blog", on)
		}
	}
	os.Unsetenv("OPINIONS")
}
