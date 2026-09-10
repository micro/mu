package google

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func calendarFixture(t *testing.T, handler calendarTransport) {
	t.Helper()
	reset()
	old := httpClient
	t.Cleanup(func() { httpClient = old; reset() })
	httpClient = &http.Client{Transport: handler}
	Store("alice", "alice@example.com", "refresh", []string{CalendarScope})
	access["alice"] = cachedToken{token: "alice", expires: time.Now().Add(time.Hour)}
}

func calendarResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestCalendarSelectionIsPrivateValidatedAndPersistent(t *testing.T) {
	calls := 0
	calendarFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer alice" {
			t.Error("wrong account credential")
		}
		if !strings.HasSuffix(r.URL.Path, "/calendarList") || r.URL.Query().Get("minAccessRole") != "reader" {
			t.Error("not listing readable calendars")
		}
		calls++
		if r.URL.Query().Get("pageToken") == "next" {
			return calendarResponse(`{"items":[{"id":"work","summary":"Work"}]}`), nil
		}
		return calendarResponse(`{"items":[{"id":"alice@example.com","summary":"Personal","primary":true}],"nextPageToken":"next"}`), nil
	})
	if err := SetCalendars("alice", []string{"alice@example.com", "work", "work"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"alice@example.com", "work"}
	if got := SelectedCalendars("alice"); !reflect.DeepEqual(got, want) || calls != 2 {
		t.Fatalf("selection %v, requests %d", got, calls)
	}
	if err := SetCalendars("alice", []string{"someone-elses-calendar"}); err == nil {
		t.Fatal("accepted calendar outside the owner's list")
	}
	if !reflect.DeepEqual(SelectedCalendars("alice"), want) {
		t.Fatal("invalid selection changed saved settings")
	}
	if err := SetCalendars("bob", []string{"work"}); err != ErrNotConnected {
		t.Fatalf("unconnected account: %v", err)
	}
	Store("alice", "alice@example.com", "new-refresh", []string{CalendarScope})
	if !reflect.DeepEqual(SelectedCalendars("alice"), want) {
		t.Fatal("reauthorization lost selection")
	}
	reset()
	Load()
	if !reflect.DeepEqual(SelectedCalendars("alice"), want) {
		t.Fatal("restart lost selection")
	}
	copy := SelectedCalendars("alice")
	copy[0] = "other"
	if !reflect.DeepEqual(SelectedCalendars("alice"), want) {
		t.Fatal("selection aliases stored state")
	}
}

func TestSelectedCalendarsMergeEventsBeforeLimitingAndShareBusySelection(t *testing.T) {
	calendarFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/freeBusy"):
			var body struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body.Items) != 2 || body.Items[0].ID != "personal" || body.Items[1].ID != "work/calendar" {
				t.Errorf("wrong busy calendars: %+v", body)
			}
			return calendarResponse(`{"calendars":{"personal":{"busy":[{"start":"2026-09-10T12:00:00Z","end":"2026-09-10T13:00:00Z"}]},"work/calendar":{"busy":[{"start":"2026-09-10T08:00:00Z","end":"2026-09-10T09:00:00Z"}]}}}`), nil
		case strings.Contains(r.URL.EscapedPath(), "work%2Fcalendar"):
			return calendarResponse(`{"items":[{"summary":"Earlier work","start":{"dateTime":"2026-09-10T08:00:00Z"}},{"summary":"Later work","start":{"dateTime":"2026-09-10T16:00:00Z"}}]}`), nil
		case strings.Contains(r.URL.Path, "/personal/events"):
			return calendarResponse(`{"items":[{"summary":"Personal","start":{"dateTime":"2026-09-10T12:00:00Z"}}]}`), nil
		default:
			t.Errorf("unexpected URL %s", r.URL)
			return calendarResponse(`{}`), nil
		}
	})
	conns["alice"].Calendars = []string{"personal", "work/calendar"}
	now := time.Now()
	entries, err := Events("alice", now, now.Add(24*time.Hour), 2)
	if err != nil || len(entries) != 2 || entries[0].Title != "Earlier work" || entries[1].Title != "Personal" {
		t.Fatalf("merged events: %+v, %v", entries, err)
	}
	busy, err := Busy("alice", now, now.Add(24*time.Hour))
	if err != nil || len(busy) != 2 {
		t.Fatalf("busy: %+v, %v", busy, err)
	}
}

func TestAnExplicitEmptySelectionSurvivesRestartAndMakesNoCalendarRequests(t *testing.T) {
	calendarFixture(t, func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/calendarList") {
			t.Error("requested an unselected calendar")
		}
		return calendarResponse(`{"items":[]}`), nil
	})
	if err := SetCalendars("alice", nil); err != nil {
		t.Fatal(err)
	}
	reset()
	Load()
	access["alice"] = cachedToken{token: "alice", expires: time.Now().Add(time.Hour)}
	if got := SelectedCalendars("alice"); got == nil || len(got) != 0 {
		t.Fatalf("empty selection reverted to default: %v", got)
	}
	now := time.Now()
	if got, err := Events("alice", now, now.Add(time.Hour), 3); err != nil || len(got) != 0 {
		t.Fatalf("events %v %v", got, err)
	}
	if got, err := Busy("alice", now, now.Add(time.Hour)); err != nil || len(got) != 0 {
		t.Fatalf("busy %v %v", got, err)
	}
}

func TestBusyDoesNotTreatCalendarErrorsAsFreeTime(t *testing.T) {
	calendarFixture(t, func(r *http.Request) (*http.Response, error) {
		return calendarResponse(`{"calendars":{"primary":{"errors":[{"reason":"notFound"}]}}}`), nil
	})
	if _, err := Busy("alice", time.Now(), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("unreadable calendar treated as free")
	}
}

func TestSharedInvitationsAppearOnceButRecurrencesRemain(t *testing.T) {
	calendarFixture(t, func(r *http.Request) (*http.Response, error) {
		return calendarResponse(`{"items":[{"iCalUID":"invite","summary":"Meeting","start":{"dateTime":"2026-09-10T12:00:00Z"}},{"iCalUID":"invite","summary":"Meeting","start":{"dateTime":"2026-09-11T12:00:00Z"}}]}`), nil
	})
	conns["alice"].Calendars = []string{"personal", "work"}
	got, err := Events("alice", time.Now(), time.Now().Add(48*time.Hour), 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("shared recurring invitation: %+v, %v", got, err)
	}
}
