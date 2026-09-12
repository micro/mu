package agent

import (
	"html"
	"mu/internal/ai"
	"mu/service/tasks"
	"strings"
	"time"
)

// activity describes work for this account, not a global online presence.
func activity(accountID, agentID string) string {
	canonical := func(id string) string {
		if id == DefaultPlatformAgent || id == DefaultSlug {
			return ""
		}
		return id
	}
	agentID = canonical(agentID)
	running, blocked, failed, queued := false, false, false, false
	for _, task := range tasks.List(accountID, "") {
		if task.Assignee != tasks.Agent || canonical(task.Agent) != agentID {
			continue
		}
		switch task.Status {
		case tasks.StatusDoing:
			running = true
		case tasks.StatusBlocked:
			blocked = true
		case tasks.StatusFailed:
			failed = true
		case tasks.StatusTodo:
			queued = true
		}
	}
	flowMu.RLock()
	var last *Flow
	for _, f := range flowStore {
		if f.AccountID != accountID || canonical(f.Agent) != agentID {
			continue
		}
		if f.Status == "running" {
			running = true
		}
		if last == nil || f.CreatedAt.After(last.CreatedAt) {
			last = f
		}
	}
	lastFailed := last != nil && last.Status == "error" && time.Since(last.CreatedAt) < 24*time.Hour
	flowMu.RUnlock()
	var states []string
	if running {
		states = append(states, "Working")
	}
	if blocked {
		states = append(states, "Needs input")
	}
	if failed || lastFailed {
		states = append(states, "Needs attention")
	}
	if queued {
		states = append(states, "Queued")
	}
	if len(states) > 0 {
		return strings.Join(states, " · ")
	}
	if !ai.Configured() {
		return "Not configured"
	}
	return "Idle"
}

func activityHTML(accountID, agentID string) string {
	return `<span class="activity-status">` + html.EscapeString(activity(accountID, agentID)) + `</span>`
}
