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

// One piece a day, and every topic reached by rotation rather than by volume.
//
// This asserted one piece per topic per day — eight a day against eight topics
// — on the argument that a topic visited every N days is not covered. The
// argument that wins is the other one: nobody publishes eight opinion pieces a
// day, and each piece is a research pass and a generation billed to the
// instance. Coverage comes from nextCategory picking the topic that has gone
// longest without one.
func TestOnePieceADayByDefault(t *testing.T) {
	if got := dailyOpinions(); got != 1 {
		t.Errorf("dailyOpinions() = %d, want 1 — eight a day is the largest "+
			"scheduled expense on the instance and no publication writes at that rate",
			got)
	}
}

// An operator can ask for more, up to the ceiling and never past the topics.
func TestTheDailyCountCanBeRaisedButIsBounded(t *testing.T) {
	t.Setenv("OPINIONS_PER_DAY", "3")
	if got := dailyOpinions(); got != 3 {
		t.Errorf("OPINIONS_PER_DAY=3 gives %d", got)
	}

	// Never more than the ceiling, whatever is asked for.
	t.Setenv("OPINIONS_PER_DAY", "500")
	if got := dailyOpinions(); got > maxDailyOpinions {
		t.Errorf("OPINIONS_PER_DAY=500 gives %d, past the ceiling of %d",
			got, maxDailyOpinions)
	}
	// And never more than there are topics — a second piece on the only
	// subject configured is that subject twice.
	if got := dailyOpinions(); got > len(opinionCategories()) {
		t.Errorf("%d pieces for %d topics", got, len(opinionCategories()))
	}

	// Nonsense is ignored rather than taken as zero.
	t.Setenv("OPINIONS_PER_DAY", "banana")
	if got := dailyOpinions(); got != 1 {
		t.Errorf("an unparseable OPINIONS_PER_DAY gives %d, want the default 1", got)
	}
}

// The topic picked is the one that has gone longest without a piece.
//
// At eight a day the picker walked the file in order and the order did not
// matter, because the whole list was covered before the day was out. At one a
// day that is the bug the old shape was hiding: the first topic in
// topics.json would win every morning and the blog would cover one subject for
// ever.
func TestTheTopicPickedIsTheOneLeftLongest(t *testing.T) {
	cats := []string{"Crypto", "Dev", "Finance"}

	// None published today. With no history at all, the first is as good as
	// any — what matters is that a topic already covered today is skipped.
	if got := nextCategory(cats, map[string]bool{"crypto": true}); got == "Crypto" {
		t.Error("a topic already published today was picked again")
	}
	if got := nextCategory(cats, map[string]bool{"crypto": true, "dev": true,
		"finance": true}); got != "" {
		t.Errorf("every topic has had one today and %q was picked anyway", got)
	}
}

// And the pieces still fit in a day.
func TestThePiecesFitInADay(t *testing.T) {
	n := dailyOpinions()
	if span := time.Duration(n) * opinionInterval(n); span > 24*time.Hour {
		t.Errorf("%d pieces at %v apart is %v of publishing — the ones at the end "+
			"never get written", n, opinionInterval(n), span)
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
