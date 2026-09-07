package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type commandTransport func(*http.Request) (*http.Response, error)

func (f commandTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCommandSearchResultsAndCache(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "test-command-key")
	old := httpClient
	calls := 0
	httpClient = &http.Client{Transport: commandTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("q") != "Sam Altman and OpenAI" || r.URL.Query().Get("count") != "5" {
			t.Errorf("wrong query: %s", r.URL.String())
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"web":{"results":[{"title":"A source","url":"https://example.com/story","description":"Useful text"}]}}`)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { httpClient = old })
	// Unique query cache entry for this test, restored so test order is irrelevant.
	braveCache.Lock()
	saved := braveCache.entries
	braveCache.entries = make(map[string]braveCacheEntry)
	braveCache.Unlock()
	t.Cleanup(func() { braveCache.Lock(); braveCache.entries = saved; braveCache.Unlock() })
	for i := 0; i < 2; i++ {
		var rsp SearchResponse
		if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "Sam Altman and OpenAI", Limit: 5}, &rsp); err != nil {
			t.Fatal(err)
		}
		if len(rsp.Items) != 1 || rsp.Items[0].Title != "A source" || rsp.Items[0].URL != "https://example.com/story" || !strings.Contains(rsp.Text, "A source") {
			t.Fatalf("lost results: %+v", rsp)
		}
	}
	if calls != 1 {
		t.Fatalf("repeated search made %d provider calls", calls)
	}
}

func TestCommandSearchFailureAndEmptyResults(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "test-command-key")
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	for _, tc := range []struct {
		name   string
		status int
		body   string
		fail   bool
	}{
		{"empty", 200, `{"web":{"results":[]}}`, false},
		{"provider error", 503, `unavailable`, true},
		{"bad json", 200, `{broken`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			httpClient = &http.Client{Transport: commandTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Status: fmt.Sprint(tc.status), Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			var rsp SearchResponse
			err := (Server{}).Search(context.Background(), &SearchRequest{Query: "command-fixture-" + tc.name}, &rsp)
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v", err)
			}
			if !tc.fail && (rsp.Text != "No web results found." || len(rsp.Items) != 0) {
				t.Fatalf("empty results: %+v", rsp)
			}
		})
	}
}
