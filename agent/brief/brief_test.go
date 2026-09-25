package brief

import (
	"testing"
	"time"
)

func TestFailedOrEmptyBriefWaitsBeforeRetry(t *testing.T) {
	oldEntries, oldAttempted, oldRunning := entries, attempted, running
	defer func() { entries, attempted, running = oldEntries, oldAttempted, oldRunning }()
	entries, running = nil, false
	attempted = time.Now()
	if due() {
		t.Fatal("a failed or empty attempt must not immediately retry")
	}
	attempted = time.Now().Add(-2 * gap)
	if !due() {
		t.Fatal("missing brief should retry after the hourly interval")
	}
	entries = []Entry{{Text: "Yesterday's news", Day: time.Now().AddDate(0, 0, -1).Format("2006-01-02"), Written: time.Now().Add(-24 * time.Hour)}}
	if Line() != "" {
		t.Fatal("yesterday's line must not appear as today's brief")
	}
	entries = []Entry{{Text: "Today's news", Day: today(), Written: time.Now()}}
	if due() {
		t.Fatal("fresh brief should not be regenerated")
	}
	if Line() != "Today's news" {
		t.Fatal("current cached summary missing")
	}
}
