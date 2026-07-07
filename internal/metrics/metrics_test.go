package metrics

import (
	"testing"
	"time"
)

func TestRecordAndSnapshot(t *testing.T) {
	Reset()

	// 10 calls to one tool, 2 of them errors, latencies 10..100ms.
	for i := 1; i <= 10; i++ {
		Record("tool:mail_read", time.Duration(i*10)*time.Millisecond, i <= 2)
	}
	Record("llm:agent-plan", 250*time.Millisecond, false)
	AddCost(7)
	AddCost(3)

	rep := Snapshot()

	if rep.Totals.Requests != 11 {
		t.Errorf("total requests = %d, want 11", rep.Totals.Requests)
	}
	if rep.Totals.Errors != 2 {
		t.Errorf("total errors = %d, want 2", rep.Totals.Errors)
	}
	if rep.Totals.CostCredits != 10 {
		t.Errorf("cost = %d, want 10", rep.Totals.CostCredits)
	}
	if rep.Saturation.Goroutines <= 0 {
		t.Errorf("goroutines = %d, want > 0", rep.Saturation.Goroutines)
	}

	// Busiest op first.
	if rep.Ops[0].Name != "tool:mail_read" {
		t.Fatalf("ops[0] = %q, want tool:mail_read", rep.Ops[0].Name)
	}
	mail := rep.Ops[0]
	if mail.Count != 10 || mail.Errors != 2 {
		t.Errorf("mail count/errors = %d/%d, want 10/2", mail.Count, mail.Errors)
	}
	if mail.ErrorRate < 0.19 || mail.ErrorRate > 0.21 {
		t.Errorf("mail error rate = %v, want ~0.2", mail.ErrorRate)
	}
	// p50 of 10..100ms (nearest-rank at index 5) is 60ms; p99 is 100ms.
	if mail.P50ms < 50 || mail.P50ms > 70 {
		t.Errorf("p50 = %vms, want ~60", mail.P50ms)
	}
	if mail.P99ms < 90 {
		t.Errorf("p99 = %vms, want ~100", mail.P99ms)
	}
}

func TestEmptySnapshot(t *testing.T) {
	Reset()
	rep := Snapshot()
	if rep.Totals.Requests != 0 || len(rep.Ops) != 0 {
		t.Errorf("empty snapshot not empty: %+v", rep.Totals)
	}
	if rep.UptimeSeconds < 0 {
		t.Errorf("negative uptime")
	}
}
