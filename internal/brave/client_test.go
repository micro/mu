package brave

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, headers http.Header) *http.Response {
	return &http.Response{StatusCode: code, Header: headers, Body: io.NopCloser(strings.NewReader(`{}`))}
}

func TestSharedSpacingAndCancellation(t *testing.T) {
	l := limiter{interval: func() time.Duration { return 10 * time.Millisecond }}
	var mu sync.Mutex
	var starts []time.Time
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return response(200, http.Header{}), nil
	})}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := http.NewRequest("GET", "https://example.test", nil)
			resp, e := l.do(client, r)
			if e != nil {
				t.Error(e)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()
	for i := 1; i < len(starts); i++ {
		if starts[i].Sub(starts[i-1]) < 9*time.Millisecond {
			t.Fatal("provider burst")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://example.test", nil)
	if _, err := l.do(client, r); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if len(starts) != 5 {
		t.Fatal("cancelled request reached provider")
	}
}

func TestRetryAndQuota(t *testing.T) {
	for _, quota := range []bool{false, true} {
		l := limiter{interval: func() time.Duration { return time.Millisecond }}
		calls := 0
		client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				h := http.Header{}
				if quota {
					h.Set("X-RateLimit-Limit", "10,100")
					h.Set("X-RateLimit-Remaining", "9,0")
					h.Set("X-RateLimit-Reset", "1,3600")
				}
				return response(429, h), nil
			}
			return response(200, http.Header{}), nil
		})}
		r, _ := http.NewRequest("GET", "https://example.test", nil)
		resp, err := l.do(client, r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if quota {
			if calls != 1 || resp.StatusCode != 429 {
				t.Fatal("retried exhausted quota")
			}
			if _, err = l.do(client, r); !errors.Is(err, ErrQuota) {
				t.Fatal("quota not shared")
			}
		} else if calls != 2 || resp.StatusCode != 200 {
			t.Fatal("short limit not retried")
		}
	}
}

func TestResetWindows(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "50,0")
	h.Set("X-RateLimit-Remaining", "49,0")
	h.Set("X-RateLimit-Reset", "1,3600")
	if d, q := resetDelay(h); d != 0 || q {
		t.Fatal("unlimited monthly quota blocked")
	}
	h.Set("Retry-After", "2")
	if d, _ := resetDelay(h); d != 2*time.Second {
		t.Fatal("Retry-After ignored")
	}
}
