package flow

import (
	"strings"
	"sync"
	"time"

	"mu/app"
)

var (
	schedulerRunning bool
	schedulerMu      sync.Mutex
)

// StartScheduler begins the background scheduler
func StartScheduler() {
	schedulerMu.Lock()
	if schedulerRunning {
		schedulerMu.Unlock()
		return
	}
	schedulerRunning = true
	schedulerMu.Unlock()

	app.Log("flow", "Scheduler started")

	go func() {
		// Check every minute
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			checkScheduledFlows()
		}
	}()
}

// checkScheduledFlows runs any flows that are due
func checkScheduledFlows() {
	now := time.Now()

	flowsMu.RLock()
	var toRun []*Flow
	for _, f := range flows {
		if !f.Enabled || f.Schedule == "" {
			continue
		}
		if shouldRun(f, now) {
			toRun = append(toRun, f)
		}
	}
	flowsMu.RUnlock()

	for _, f := range toRun {
		app.Log("flow", "Running scheduled flow: %s", f.Name)
		go func(flow *Flow) {
			result := Execute(flow, flow.UserID)
			if result.Success {
				app.Log("flow", "Flow %s completed successfully", flow.Name)
			} else {
				app.Log("flow", "Flow %s failed: %s", flow.Name, result.Error)
			}
		}(f)
	}
}

// shouldRun checks if a flow should run based on its cron schedule and current time
func shouldRun(f *Flow, now time.Time) bool {
	// Simple cron parsing: "minute hour day month weekday"
	// We only support: "0 H * * *" (daily at hour H) and "0 * * * *" (hourly)
	
	parts := strings.Fields(f.Schedule)
	if len(parts) != 5 {
		return false
	}

	minute := parts[0]
	hour := parts[1]
	weekday := parts[4]

	// Only run at minute 0 (or whatever minute is specified)
	if minute != "*" && minute != itoa(now.Minute()) {
		return false
	}

	// Check hour
	if hour != "*" && hour != itoa(now.Hour()) {
		return false
	}

	// Check weekday (0=Sunday, 1=Monday, etc.)
	if weekday != "*" {
		nowWeekday := int(now.Weekday())
		if weekday != itoa(nowWeekday) {
			return false
		}
	}

	// Don't run more than once per minute - check last run
	if !f.LastRun.IsZero() {
		sinceLastRun := now.Sub(f.LastRun)
		if sinceLastRun < 1*time.Minute {
			return false
		}
	}

	return true
}
