package app

import (
	"testing"
	"time"
)

func TestStartupTimingsSnapshot(t *testing.T) {
	startupTimings.Lock()
	oldSteps, oldTotal := startupTimings.steps, startupTimings.total
	startupTimings.steps, startupTimings.total = nil, 0
	startupTimings.Unlock()
	t.Cleanup(func() {
		startupTimings.Lock()
		defer startupTimings.Unlock()
		startupTimings.steps, startupTimings.total = oldSteps, oldTotal
	})
	RecordStartup("data.Load", 6*time.Second)
	CompleteStartup(42 * time.Second)
	steps, total := StartupTimings()
	if len(steps) != 1 || total != 42*time.Second || steps[0].Duration != 6*time.Second {
		t.Fatal("startup timing was lost")
	}
	steps[0].Component = "changed"
	again, _ := StartupTimings()
	if again[0].Component != "data.Load" {
		t.Fatal("reader can mutate recorded timings")
	}
}
