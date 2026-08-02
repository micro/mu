package video

import (
	"strings"
	"testing"
	"time"
)

func resetSearchLimits() {
	searchMu.Lock()
	defer searchMu.Unlock()
	perAccount = map[string]*searchBucket{}
	globalBucket = &searchBucket{}
}

// The per-account cap stops one caller taking the instance's whole day.
func TestSearchLimitIsPerAccount(t *testing.T) {
	resetSearchLimits()
	t.Setenv("VIDEO_SEARCH_PER_HOUR", "3")
	t.Setenv("VIDEO_SEARCH_PER_DAY", "100")

	for i := 0; i < 3; i++ {
		if err := allowSearch("alice"); err != nil {
			t.Fatalf("search %d refused: %v", i+1, err)
		}
	}
	if err := allowSearch("alice"); err == nil {
		t.Error("alice is over her hourly cap and was allowed anyway")
	}
	// Bob is unaffected: the cap is hers, not the instance's.
	if err := allowSearch("bob"); err != nil {
		t.Errorf("bob was refused because alice searched: %v", err)
	}
}

// The global cap is what keeps the instance inside YouTube's daily quota. It
// is the reason a price was the wrong tool: the quota is shared, so spending
// it has to be limited for everyone, not billed to whoever is holding it.
func TestSearchLimitProtectsTheSharedQuota(t *testing.T) {
	resetSearchLimits()
	t.Setenv("VIDEO_SEARCH_PER_HOUR", "100")
	t.Setenv("VIDEO_SEARCH_PER_DAY", "2")

	if err := allowSearch("alice"); err != nil {
		t.Fatal(err)
	}
	if err := allowSearch("bob"); err != nil {
		t.Fatal(err)
	}
	err := allowSearch("carol")
	if err == nil {
		t.Fatal("the instance is over its daily quota and carol was allowed anyway")
	}
	if !strings.Contains(err.Error(), "day") {
		t.Errorf("the refusal should say it is the instance's daily limit, got %q", err)
	}
}

// A refused search must not spend quota, or a caller hammering a limit they
// have already hit would burn the day's allowance being told no.
func TestRefusedSearchSpendsNothing(t *testing.T) {
	resetSearchLimits()
	t.Setenv("VIDEO_SEARCH_PER_HOUR", "1")
	t.Setenv("VIDEO_SEARCH_PER_DAY", "10")

	if err := allowSearch("alice"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := allowSearch("alice"); err == nil {
			t.Fatal("alice is over her cap and was allowed")
		}
	}

	searchMu.Lock()
	spent := globalBucket.count
	searchMu.Unlock()
	if spent != 1 {
		t.Errorf("five refusals spent %d of the shared quota, want 1", spent)
	}
}

// The hourly window rolls, so a cap is a pause and not a lockout.
func TestSearchLimitWindowExpires(t *testing.T) {
	resetSearchLimits()
	t.Setenv("VIDEO_SEARCH_PER_HOUR", "1")
	t.Setenv("VIDEO_SEARCH_PER_DAY", "10")

	if err := allowSearch("alice"); err != nil {
		t.Fatal(err)
	}
	if err := allowSearch("alice"); err == nil {
		t.Fatal("alice should be capped")
	}

	searchMu.Lock()
	perAccount["alice"].resetAt = time.Now().Add(-time.Minute)
	searchMu.Unlock()

	if err := allowSearch("alice"); err != nil {
		t.Errorf("alice is still capped after her window passed: %v", err)
	}
}
