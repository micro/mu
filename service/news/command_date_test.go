package news

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewsTodayUsesLocalDateAndExcludesFuture(t *testing.T) {
	at := time.Date(2026, 9, 9, 0, 30, 0, 0, time.FixedZone("home", 14*3600))
	today := &Post{Title: "Today", PostedAt: at.Add(-15 * time.Minute)}
	yesterday := &Post{Title: "Yesterday", PostedAt: at.Add(-time.Hour)}
	future := &Post{Title: "Future", PostedAt: at.Add(time.Hour)}
	got := newsForDay([]*Post{nil, yesterday, today, future, {Title: "Undated"}}, at)
	if len(got) != 1 || got[0] != today {
		t.Fatalf("wrong calendar day: %+v", got)
	}
	text := headlinesText(got, "", 5, at)
	if !strings.Contains(text, "2026-09-09") || strings.Contains(text, "2026-09-08") {
		t.Fatalf("contradictory date: %s", text)
	}
	var rsp ListResponse
	if err := (Server{}).List(context.Background(), &ListRequest{Day: "tomorrow"}, &rsp); err == nil {
		t.Fatal("unsupported date ignored")
	}
}
