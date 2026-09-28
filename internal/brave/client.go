// Package brave coordinates provider requests across web and image search.
// Limits are shared by one Mu process; replicas sharing a key must divide the
// allowance between instances or use a shared outbound gateway.
package brave

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/settings"
)

var ErrQuota = errors.New("Brave search quota exhausted; check the provider account")
var shared = limiter{interval: requestInterval}

type limiter struct {
	once             sync.Once
	gate             chan struct{}
	mu               sync.Mutex
	next, quotaUntil time.Time
	interval         func() time.Duration
}

func requestInterval() time.Duration {
	rps, err := strconv.Atoi(settings.Get("BRAVE_SEARCH_RPS"))
	if err != nil || rps < 1 || rps > 50 {
		rps = 10
	}
	// Headroom for the provider's sliding window.
	return time.Second/time.Duration(rps) + time.Millisecond
}

func (l *limiter) wait(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		now := time.Now()
		if now.Before(l.quotaUntil) {
			l.mu.Unlock()
			return ErrQuota
		}
		delay := l.next.Sub(now)
		if delay <= 0 {
			l.next = now.Add(l.interval())
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Do spaces all outgoing requests and retries a short 429 once. Other failures
// and exhausted monthly quotas are returned without repeated provider calls.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	return shared.do(client, req)
}

func (l *limiter) do(client *http.Client, req *http.Request) (*http.Response, error) {
	l.once.Do(func() { l.gate = make(chan struct{}, 1) })
	waitCtx, cancel := context.WithTimeout(req.Context(), 20*time.Second)
	defer cancel()
	select {
	case l.gate <- struct{}{}:
	case <-waitCtx.Done():
		return nil, waitCtx.Err()
	}
	defer func() { <-l.gate }()
	for attempt := 0; ; attempt++ {
		if err := l.wait(req.Context()); err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		// Space from receipt of headers so scheduler/network delays cannot
		// turn already-reserved slots into an outbound burst.
		l.mu.Lock()
		l.next = time.Now().Add(l.interval())
		l.mu.Unlock()
		delay, quota := resetDelay(resp.Header)
		if resp.StatusCode == http.StatusTooManyRequests {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			if err != nil {
				return nil, err
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
			var payload struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(body, &payload)
			quota = quota || payload.Error.Code == "QUOTA_LIMITED"
			if quota && delay < time.Minute {
				delay = time.Minute
			}
			if delay < l.interval() {
				delay = l.interval()
			}
		}
		if delay > 0 {
			l.mu.Lock()
			until := time.Now().Add(delay)
			if until.After(l.next) {
				l.next = until
			}
			if quota {
				l.quotaUntil = until
			}
			l.mu.Unlock()
		}
		if resp.StatusCode != http.StatusTooManyRequests || quota || attempt >= 1 || delay > 5*time.Second || req.Method != http.MethodGet {
			return resp, nil
		}
		resp.Body.Close()
	}
}

// Brave reports parallel per-second and monthly windows. Only exhausted
// windows matter; an unlimited (zero-limit) monthly window is not exhausted.
func resetDelay(h http.Header) (time.Duration, bool) {
	delay := time.Duration(0)
	quota := false
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 && secs <= 31536000 {
			delay = time.Duration(secs) * time.Second
		} else if when, err := http.ParseTime(v); err == nil {
			delay = time.Until(when)
		}
	}
	remaining := strings.Split(h.Get("X-RateLimit-Remaining"), ",")
	limits := strings.Split(h.Get("X-RateLimit-Limit"), ",")
	resets := strings.Split(h.Get("X-RateLimit-Reset"), ",")
	for i, v := range remaining {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n != 0 || i >= len(resets) {
			continue
		}
		if i < len(limits) && strings.TrimSpace(limits[i]) == "0" {
			continue
		}
		secs, err := strconv.Atoi(strings.TrimSpace(resets[i]))
		if err != nil || secs <= 0 || secs > 31536000 {
			continue
		}
		d := time.Duration(secs) * time.Second
		if d > delay {
			delay = d
		}
		if i > 0 {
			quota = true
		}
	}
	return delay, quota
}
