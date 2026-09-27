package usage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestActivityRetainsFailuresSeparatelyAndBoundsHistory(t *testing.T) {
	mu.Lock()
	old := rings
	rings = newStore()
	mu.Unlock()
	defer func() { mu.Lock(); rings = old; mu.Unlock() }()
	RecordActivity(Activity{Surface: "mcp", Operation: "web_search", Account: "asim", TokenID: "public-id", Outcome: FailureKind(402, "insufficient credits: secret-body")})
	for i := 0; i < maxActivity+1; i++ {
		RecordActivity(Activity{Surface: "mcp", Operation: "news_search", Account: "other", Outcome: "ok"})
	}
	if got := Activities(false); len(got) != maxActivity {
		t.Fatal(len(got))
	}
	failures := Activities(true)
	if len(failures) != 1 || failures[0].Account != "asim" || failures[0].Outcome != "credits or quota" {
		t.Fatal(failures)
	}
	b, _ := json.Marshal(failures)
	if strings.Contains(string(b), "secret-body") {
		t.Fatal("error contents retained")
	}
	RecordActivity(Activity{At: time.Now().AddDate(0, 0, -15), Outcome: "timeout"})
	if len(Activities(true)) != 1 {
		t.Fatal("expired failure shown")
	}
	Save()
	restore()
	if len(Activities(false)) != maxActivity-1 || len(Activities(true)) != 1 {
		t.Fatal("activity did not survive restore")
	}
}

func TestAnonymousNotFoundStaysInActivity(t *testing.T) {
	mu.Lock()
	old := rings
	rings = newStore()
	mu.Unlock()
	defer func() { mu.Lock(); rings = old; mu.Unlock() }()
	for _, e := range []Activity{{Surface: "http", Operation: "GET robots.txt", Status: 404, Outcome: "request failed"}, {Surface: "http", Account: "asim", Operation: "GET docs", Status: 404, Outcome: "request failed"}, {Surface: "http", Operation: "GET blog", Status: 500, Outcome: "service failure"}, {Surface: "http", Operation: "GET social", Status: 403, Outcome: "access blocked"}} {
		RecordActivity(e)
	}
	if len(Activities(false)) != 4 || len(Activities(true)) != 3 {
		t.Fatal("missing-page filtering hid actionable failures")
	}
	// Existing stored records receive the same treatment after upgrading.
	mu.Lock()
	rings.Failures = append(rings.Failures, Activity{At: time.Now(), Surface: "http", Account: "guest", Status: 404, Outcome: "request failed"})
	mu.Unlock()
	if len(Activities(true)) != 3 {
		t.Fatal("historical anonymous 404 remained in Errors")
	}
}
