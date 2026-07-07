// Package metrics is Mu's in-process runtime fitness recorder. It is the
// interface the operator loop reads (see internal/docs/OPERATOR.md): the four
// golden signals — latency, traffic, errors, saturation — plus cost, digested
// from live behaviour rather than synthetic tests.
//
// It is deliberately tiny and dependency-free: any package may Record without an
// import cycle, and a Snapshot is cheap enough to serve on request. It keeps a
// bounded ring of recent latencies per operation for percentiles and cumulative
// counters since process start.
package metrics

import (
	"runtime"
	"sort"
	"sync"
	"time"
)

// sampleCap is how many recent latency samples we keep per operation for
// percentiles. Bounded so memory is O(ops × sampleCap) regardless of traffic.
const sampleCap = 512

var startTime = time.Now()

type opStat struct {
	count   int64
	errors  int64
	samples []time.Duration
	next    int
	full    bool
}

func (s *opStat) observe(d time.Duration, isErr bool) {
	s.count++
	if isErr {
		s.errors++
	}
	s.samples[s.next] = d
	s.next = (s.next + 1) % sampleCap
	if s.next == 0 {
		s.full = true
	}
}

// valid returns the populated latency samples (a copy is not needed; the caller
// sorts its own slice).
func (s *opStat) valid() []time.Duration {
	if s.full {
		out := make([]time.Duration, sampleCap)
		copy(out, s.samples)
		return out
	}
	out := make([]time.Duration, s.next)
	copy(out, s.samples[:s.next])
	return out
}

var (
	mu          sync.Mutex
	ops         = map[string]*opStat{}
	costCredits int64
)

// Record logs one observation of a named operation: how long it took and
// whether it failed. Safe for concurrent use and O(1). Use a stable, low
// cardinality name (e.g. "tool:mail_read", "llm:agent-plan") — never embed user
// input, ids, or free text, or the map grows unbounded.
func Record(op string, d time.Duration, isErr bool) {
	if op == "" {
		return
	}
	mu.Lock()
	s := ops[op]
	if s == nil {
		s = &opStat{samples: make([]time.Duration, sampleCap)}
		ops[op] = s
	}
	s.observe(d, isErr)
	mu.Unlock()
}

// AddCost accumulates spend (in credits) attributable to serving requests, for
// the cost signal. No-op for non-positive values.
func AddCost(credits int) {
	if credits <= 0 {
		return
	}
	mu.Lock()
	costCredits += int64(credits)
	mu.Unlock()
}

// OpReport is the golden-signal summary for one operation.
type OpReport struct {
	Name      string  `json:"name"`
	Count     int64   `json:"count"`
	Errors    int64   `json:"errors"`
	ErrorRate float64 `json:"error_rate"` // 0..1
	P50ms     float64 `json:"p50_ms"`
	P95ms     float64 `json:"p95_ms"`
	P99ms     float64 `json:"p99_ms"`
}

// Saturation is the process resource snapshot.
type Saturation struct {
	Goroutines int    `json:"goroutines"`
	AllocMB    uint64 `json:"alloc_mb"`
	SysMB      uint64 `json:"sys_mb"`
	NumGC      uint32 `json:"num_gc"`
}

// Totals aggregates every recorded operation.
type Totals struct {
	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	ErrorRate   float64 `json:"error_rate"` // 0..1
	CostCredits int64   `json:"cost_credits"`
}

// Report is the runtime fitness digest the operator reads.
type Report struct {
	GeneratedAt   time.Time  `json:"generated_at"`
	UptimeSeconds float64    `json:"uptime_seconds"`
	Totals        Totals     `json:"totals"`
	Saturation    Saturation `json:"saturation"`
	Ops           []OpReport `json:"ops"`
}

// Snapshot builds the current digest. Percentiles are over recent samples; counts
// and cost are cumulative since process start.
func Snapshot() Report {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rep := Report{
		GeneratedAt:   time.Now().UTC(),
		UptimeSeconds: time.Since(startTime).Seconds(),
		Saturation: Saturation{
			Goroutines: runtime.NumGoroutine(),
			AllocMB:    m.Alloc / (1 << 20),
			SysMB:      m.Sys / (1 << 20),
			NumGC:      m.NumGC,
		},
	}

	mu.Lock()
	rep.Totals.CostCredits = costCredits
	for name, s := range ops {
		o := OpReport{Name: name, Count: s.count, Errors: s.errors}
		if s.count > 0 {
			o.ErrorRate = float64(s.errors) / float64(s.count)
		}
		v := s.valid()
		o.P50ms = percentileMs(v, 0.50)
		o.P95ms = percentileMs(v, 0.95)
		o.P99ms = percentileMs(v, 0.99)
		rep.Ops = append(rep.Ops, o)
		rep.Totals.Requests += s.count
		rep.Totals.Errors += s.errors
	}
	mu.Unlock()

	if rep.Totals.Requests > 0 {
		rep.Totals.ErrorRate = float64(rep.Totals.Errors) / float64(rep.Totals.Requests)
	}
	// Busiest operations first — that's where fitness pressure concentrates.
	sort.Slice(rep.Ops, func(i, j int) bool { return rep.Ops[i].Count > rep.Ops[j].Count })
	return rep
}

// percentileMs returns the p-th percentile (0..1) of the samples in ms, using
// nearest-rank. Zero when there are no samples.
func percentileMs(samples []time.Duration, p float64) float64 {
	n := len(samples)
	if n == 0 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	rank := int(p * float64(n))
	if rank >= n {
		rank = n - 1
	}
	return float64(samples[rank].Microseconds()) / 1000.0
}

// Reset clears all recorded state. For tests only.
func Reset() {
	mu.Lock()
	ops = map[string]*opStat{}
	costCredits = 0
	mu.Unlock()
}
