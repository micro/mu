// Package exercise is Mu's synthetic monitor — the operator loop's exerciser
// (see internal/docs/OPERATOR.md). It drives a fixed battery of representative
// requests at a running Mu and reports availability and latency, so there is a
// fitness signal even when real traffic is thin, and so a release can be judged
// by comparing the battery against a canary and stable target.
//
// This is ordinary black-box synthetic monitoring (think an external uptime
// probe), not anything novel: hit real endpoints, time them, check they succeed.
package exercise

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Probe is one synthetic request and its success check.
type Probe struct {
	Name   string
	Method string
	Path   string
	Body   string
	Header map[string]string
	// Check returns nil when the response is considered healthy.
	Check func(status int, body []byte) error
}

// Result is the outcome of running one probe (possibly repeated).
type Result struct {
	Name      string  `json:"name"`
	Runs      int     `json:"runs"`
	OK        int     `json:"ok"`
	Fail      int     `json:"fail"`
	P50ms     float64 `json:"p50_ms"`
	P95ms     float64 `json:"p95_ms"`
	P99ms     float64 `json:"p99_ms"`
	LastError string  `json:"last_error,omitempty"`
}

// Report is the exerciser's fitness artifact for one target.
type Report struct {
	Target     string    `json:"target"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs float64   `json:"duration_ms"`
	Runs       int       `json:"runs"`
	Passed     int       `json:"passed"` // probes with zero failures
	Failed     int       `json:"failed"` // probes with any failure
	Results    []Result  `json:"results"`
}

// Healthy reports whether every probe passed all runs.
func (r Report) Healthy() bool { return r.Failed == 0 }

// Battery is the default set of probes: cheap, public, deterministic GETs that
// measure the app's availability and latency without incurring LLM cost. With
// deep=true it adds a guest agent query that exercises the tool/LLM path (and so
// populates /internal/health) — but that costs tokens and is guest-rate-limited,
// so it is for occasional canary checks, not continuous polling.
func Battery(deep bool) []Probe {
	ok200 := func(status int, _ []byte) error {
		if status != http.StatusOK {
			return fmt.Errorf("status %d", status)
		}
		return nil
	}
	probes := []Probe{
		{Name: "status", Method: "GET", Path: "/status?format=json", Check: func(s int, b []byte) error {
			if s != http.StatusOK {
				return fmt.Errorf("status %d", s)
			}
			if !strings.Contains(string(b), "healthy") {
				return fmt.Errorf("no health field in body")
			}
			return nil
		}},
		{Name: "landing", Method: "GET", Path: "/", Check: ok200},
		{Name: "news", Method: "GET", Path: "/news", Check: ok200},
		{Name: "markets", Method: "GET", Path: "/markets", Check: ok200},
		{Name: "mcp", Method: "GET", Path: "/mcp", Check: ok200},
		{Name: "api", Method: "GET", Path: "/api", Check: ok200},
	}
	if deep {
		probes = append(probes, Probe{
			Name:   "agent",
			Method: "POST",
			Path:   "/agent",
			Body:   `{"prompt":"say hello in one word","model":"standard"}`,
			Header: map[string]string{"Content-Type": "application/json"},
			Check: func(s int, b []byte) error {
				// A guest query may be rate-limited (401) — treat that as a
				// reachable endpoint, not an outage.
				if s == http.StatusOK || s == http.StatusUnauthorized {
					return nil
				}
				return fmt.Errorf("status %d", s)
			},
		})
	}
	return probes
}

// Run executes each probe runs times against baseURL and returns a Report.
// runs<1 is treated as 1. A nil client uses a sane default.
func Run(ctx context.Context, baseURL string, probes []Probe, runs int, client *http.Client) Report {
	if runs < 1 {
		runs = 1
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	base := strings.TrimRight(baseURL, "/")
	rep := Report{Target: base, StartedAt: time.Now().UTC(), Runs: runs}
	started := time.Now()

	for _, p := range probes {
		res := Result{Name: p.Name, Runs: runs}
		var samples []time.Duration
		for i := 0; i < runs; i++ {
			d, err := runOnce(ctx, client, base, p)
			samples = append(samples, d)
			if err != nil {
				res.Fail++
				res.LastError = err.Error()
			} else {
				res.OK++
			}
		}
		res.P50ms = percentileMs(samples, 0.50)
		res.P95ms = percentileMs(samples, 0.95)
		res.P99ms = percentileMs(samples, 0.99)
		rep.Results = append(rep.Results, res)
		if res.Fail == 0 {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}
	rep.DurationMs = float64(time.Since(started).Microseconds()) / 1000.0
	return rep
}

func runOnce(ctx context.Context, client *http.Client, base string, p Probe) (time.Duration, error) {
	var bodyReader io.Reader
	if p.Body != "" {
		bodyReader = strings.NewReader(p.Body)
	}
	req, err := http.NewRequestWithContext(ctx, p.Method, base+p.Path, bodyReader)
	if err != nil {
		return 0, err
	}
	for k, v := range p.Header {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return time.Since(start), err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	d := time.Since(start)
	if p.Check != nil {
		if err := p.Check(resp.StatusCode, body); err != nil {
			return d, err
		}
	}
	return d, nil
}

func percentileMs(samples []time.Duration, pct float64) float64 {
	n := len(samples)
	if n == 0 {
		return 0
	}
	s := make([]time.Duration, n)
	copy(s, samples)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	rank := int(pct * float64(n))
	if rank >= n {
		rank = n - 1
	}
	return float64(s[rank].Microseconds()) / 1000.0
}
