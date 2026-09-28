package web

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestConcurrentSearchesShareProviderResult(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "test-key")
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	httpClient = &http.Client{Transport: searchTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"web":{"results":[{"title":"Result"}]}}`))}, nil
	})}
	var wg sync.WaitGroup
	search := func() {
		defer wg.Done()
		rows, err := searchBraveCached(context.Background(), "shared-flight-test", 6, braveCacheTTL)
		if err != nil || len(rows) != 1 {
			t.Errorf("result %v %v", rows, err)
		}
	}
	wg.Add(1)
	go search()
	<-entered
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go search()
	}
	// Requests arriving after the flight finishes must use the cache too.
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("made %d duplicate requests", calls.Load())
	}
	_, err := searchBraveCached(context.Background(), "shared-flight-test", 10, time.Minute)
	if err != nil || calls.Load() != 2 {
		t.Fatal("different result limits shared an undersized cache entry")
	}
}
