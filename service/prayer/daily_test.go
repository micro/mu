package prayer

import (
	"fmt"
	"mu/internal/data"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDailyReminderRejectsStaleCacheAndResponse(t *testing.T) {
	const date = "2040-10-14"
	data.SaveFile("reminder_daily_v2_"+date+".json", `{"date":"2040-10-13","verse":"Yesterday"}`)
	responseDate := "2040-10-13"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/"+date || r.URL.RawQuery != "" {
			t.Error("missing requested date")
		}
		fmt.Fprintf(w, `{"date":%q,"verse":"Today"}`, responseDate)
	}))
	defer server.Close()
	if got := dailyReminderForDate(date, server.URL); got != nil {
		t.Fatal("stale reminder accepted")
	}
	responseDate = date
	if got := dailyReminderForDate(date, server.URL); got == nil || got.Verse != "Today" {
		t.Fatal("correct daily reminder missing")
	}
	if got := dailyReminderForDate(date, server.URL); got == nil || requests != 2 {
		t.Fatal("dated cache not reused")
	}
}

func TestDailyReminderDoesNotUseLatestOnFailure(t *testing.T) {
	date := time.Now().Format("2006-01-02")
	data.SaveFile("reminder_daily_v2_"+date+".json", `{}`)
	data.SaveFile("reminder.json", `{"verse":"Stale rotating reminder"}`)
	if got := dailyReminderForDate(date, "http://127.0.0.1:0"); got != nil {
		t.Fatal("fell back to unrelated reminder")
	}
}

func TestDatedReminderPageKeepsEveryTabOnSameDay(t *testing.T) {
	const date = "2041-01-02"
	rd := &ReminderData{Date: date, Verse: "Exact verse", Message: "Exact reflection", Hadith: "Exact hadith", Name: "Exact name"}
	for _, view := range []string{"verse", "saying", "name", "reflection"} {
		page := renderPrayerPage(rd, view, date)
		for _, tab := range []string{"verse", "saying", "name", "reflection", "times"} {
			if !strings.Contains(page, "view="+tab+"&amp;date="+date) && !strings.Contains(page, "view="+tab+"&date="+date) {
				t.Fatal("tab lost date", page)
			}
		}
	}
	page := renderPrayerPage(rd, "reflection", date)
	if !strings.Contains(page, rd.Message) || strings.Contains(page, rd.Verse) {
		t.Fatal("reflection view changed source")
	}
}
