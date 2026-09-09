package app

import (
	"sync"
	"time"
)

// StartupTiming describes initialization of this process, not a previous run.
type StartupTiming struct {
	Component string
	Duration  time.Duration
}

var startupTimings struct {
	sync.RWMutex
	steps []StartupTiming
	total time.Duration
}

func RecordStartup(component string, elapsed time.Duration) {
	startupTimings.Lock()
	defer startupTimings.Unlock()
	startupTimings.steps = append(startupTimings.steps, StartupTiming{component, elapsed})
}

func CompleteStartup(elapsed time.Duration) {
	startupTimings.Lock()
	defer startupTimings.Unlock()
	startupTimings.total = elapsed
}

func StartupTimings() ([]StartupTiming, time.Duration) {
	startupTimings.RLock()
	defer startupTimings.RUnlock()
	return append([]StartupTiming(nil), startupTimings.steps...), startupTimings.total
}
