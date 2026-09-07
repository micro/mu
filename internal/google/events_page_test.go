package google

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type calendarTransport func(*http.Request) (*http.Response, error)

func (f calendarTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEventsFollowProviderPages(t *testing.T) {
	reset()
	mu.Lock()
	access["calendar-pages"] = cachedToken{token: "fixture", expires: time.Now().Add(time.Hour)}
	mu.Unlock()
	old := httpClient
	t.Cleanup(func() { httpClient = old; reset() })
	for _, limit := range []int{0, 27} {
		calls := 0
		httpClient = &http.Client{Transport: calendarTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			q := r.URL.Query()
			if q.Get("timeMin") == "" || q.Get("timeMax") == "" || q.Get("singleEvents") != "true" {
				t.Fatal("lost calendar window")
			}
			start, end, next := 0, 25, "second"
			if calls == 2 {
				if q.Get("pageToken") != "second" {
					t.Fatal("missing continuation")
				}
				start, end, next = 25, 30, ""
			} else if calls != 1 {
				t.Fatal("unexpected provider request")
			}
			var items []any
			for i := start; i < end; i++ {
				items = append(items, map[string]any{"summary": fmt.Sprint(i), "start": map[string]string{"dateTime": "2026-09-07T10:00:00Z"}, "end": map[string]string{"dateTime": "2026-09-07T11:00:00Z"}})
			}
			raw, _ := json.Marshal(map[string]any{"items": items, "nextPageToken": next})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
		})}
		now := time.Now()
		entries, err := Events("calendar-pages", now, now.Add(14*24*time.Hour), limit)
		want := 30
		if limit > 0 {
			want = limit
		}
		if err != nil || len(entries) != want || calls != 2 {
			t.Fatalf("limit %d: %d entries, %d requests, %v", limit, len(entries), calls, err)
		}
	}
}

func TestEventsRejectRepeatedContinuation(t *testing.T) {
	reset()
	mu.Lock()
	access["calendar-repeat"] = cachedToken{token: "fixture", expires: time.Now().Add(time.Hour)}
	mu.Unlock()
	old := httpClient
	t.Cleanup(func() { httpClient = old; reset() })
	calls := 0
	httpClient = &http.Client{Transport: calendarTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 2 {
			t.Fatal("looped on repeated token")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[],"nextPageToken":"same"}`)), Header: make(http.Header)}, nil
	})}
	now := time.Now()
	if _, err := Events("calendar-repeat", now, now.Add(time.Hour), 0); err == nil {
		t.Fatal("accepted repeated page token")
	}
}
