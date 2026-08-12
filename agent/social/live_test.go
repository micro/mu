package social

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mu/internal/ai"
)

// TestAgainstTheRealFirehose is a probe, not a test: it runs only when somebody
// asks for it, because it needs the network and it needs a minute.
//
//	SOCIAL_LIVE=1 go test ./agent/social/ -run TestAgainstTheRealFirehose -v
//
// It exists because the filters are guesses about a stream nobody can see from
// a unit test — how much survives, whether the categories match anything real,
// and whether what comes out is worth reading. That is answerable by looking,
// and every look so far has found something no unit test would have.
func TestAgainstTheRealFirehose(t *testing.T) {
	if os.Getenv("SOCIAL_LIVE") == "" {
		t.Skip("set SOCIAL_LIVE=1 to watch the real network")
	}
	t.Setenv("SOCIAL_ATPROTO", "true")

	surfaced := 0
	Surface = func(c *candidate) {
		surfaced++
		t.Logf("[%s] score %d  %s\n    %s", c.Category, c.Score, c.host(), c.display())
	}
	defer func() { Surface = func(*candidate) {} }()

	// One window, then one review — rather than Watch, which would sleep out
	// the rest of the fifteen minutes.
	seen, kept, err := sample()
	if err != nil {
		t.Fatalf("the firehose is unreachable from here: %v", err)
	}

	mu.Lock()
	short := len(shortlist(candidates))
	mu.Unlock()
	review()

	t.Logf("saw %d posts, %d passed the filters, %d on the shortlist, %d surfaced",
		seen, kept, short, surfaced)
	if seen == 0 {
		t.Fatal("no events at all")
	}
	if short > 1 && surfaced == 0 {
		t.Errorf("%d on the shortlist and a review surfaced none of them", short)
	}
}

// TestTheJudgeRefusesWhatTheScorerCannotSee — the three posts below are a real
// shortlist from a real window, and the arithmetic surfaced all three: an
// Economist piece, another bot's error message, and a bot that posts railway
// stations. Nothing measurable separates them. That is the whole argument for
// the model call, so it is worth asserting rather than assuming.
//
//	ATLAS_API_KEY=… SOCIAL_LIVE=1 go test ./agent/social/ -run TestTheJudge -v
func TestTheJudgeRefusesWhatTheScorerCannotSee(t *testing.T) {
	if os.Getenv("SOCIAL_LIVE") == "" {
		t.Skip("set SOCIAL_LIVE=1 to ask a real model")
	}
	if !ai.Configured() {
		t.Skip("no model configured")
	}
	short := []*candidate{
		{Category: "World", Link: "https://www.economist.com/finance-and-economics/x",
			Text: "'China is innovative. Its economy is a mess. Which matters more?' At no time in " +
				"modern history has a large country gone all in on investment in high-end tech while " +
				"also navigating a slowing economy & a local-govt debt crisis, notes Yuen Yuen Ang " +
				"of Johns Hopkins"},
		{Category: "Tech", Link: "https://llama.app",
			Text: "💡 llama.cpp\nGroq was unavailable, so this is a static fallback with no " +
				"LLM-generated stance\n🔗 https://llama.app"},
		{Category: "UK", Link: "https://en.wikipedia.org/wiki/Bentley_railway_station",
			Text: "Bentley railway station, railway station in Bentley, East Hampshire, England, " +
				"UK. #trains #TrainStations #map"},
	}
	got := judge(short)
	for _, c := range got {
		t.Logf("kept: [%s] %.80s", c.Category, c.Text)
	}
	if len(got) == 0 {
		t.Fatal("the judge kept nothing at all — the Economist piece is the one good post here")
	}
	if got[0] != short[0] {
		t.Errorf("first pick was %q, want the Economist piece", got[0].host())
	}
	for _, c := range got[1:] {
		t.Errorf("kept something the scorer could not tell from the good one: %.80s", c.Text)
	}
}

// TestTheFastPathAgreesWithTheSlowOne. worthParsing refuses on raw bytes what
// consider refuses on a parsed event, so the two can disagree — and a
// disagreement is invisible, because the post it wrongly drops never reaches
// anything that could notice. This checks them against each other on live
// traffic, which is the only place the shapes are real.
//
//	SOCIAL_LIVE=1 go test ./agent/social/ -run TestTheFastPath -v
func TestTheFastPathAgreesWithTheSlowOne(t *testing.T) {
	if os.Getenv("SOCIAL_LIVE") == "" {
		t.Skip("set SOCIAL_LIVE=1 to watch the real network")
	}
	conn, _, err := websocket.DefaultDialer.Dial(
		jetstreamHost+"?wantedCollections=app.bsky.feed.post", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(45 * time.Second)) //nolint:errcheck
	total, fast, wrongly := 0, 0, 0
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		total++
		quick := worthParsing(msg)
		if quick {
			fast++
		}
		var ev event
		if json.Unmarshal(msg, &ev) != nil {
			continue
		}
		// The only direction that matters: the fast path must never drop
		// something the real filter would have kept.
		if !quick && consider(ev) != nil {
			wrongly++
			t.Errorf("worthParsing dropped a candidate: %.200s", msg)
		}
	}
	if total == 0 {
		t.Fatal("no events at all")
	}
	t.Logf("%d messages, %d reached the parser (%d%%), %d wrongly dropped",
		total, fast, fast*100/total, wrongly)
}
