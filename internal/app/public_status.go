package app

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

// Public status exposes aggregate observations, never provider errors or user data.
type CapabilityHealth struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Details string `json:"details"`
}

type PublicStatusResponse struct {
	State        string             `json:"state"`
	CheckedAt    time.Time          `json:"checked_at"`
	Capabilities []CapabilityHealth `json:"capabilities"`
}

func checkPublicStatus() PublicStatusResponse {
	return publicStatusAt(time.Now(), APILog())
}

func publicStatusAt(now time.Time, entries []*APILogEntry) PublicStatusResponse {
	model := CapabilityHealth{"Assistant models", "unknown", "No recent completed model calls."}
	var durations []time.Duration
	failed := 0
	for _, e := range entries {
		if e == nil || e.Kind != "model" || e.Time.Before(now.Add(-15*time.Minute)) || e.Time.After(now) {
			continue
		}
		bad := e.Error != "" || e.ErrorKind != "" || e.Status >= 400
		if !bad && e.Outcome != "done" && e.Outcome != "completed" && e.Outcome != "success" {
			continue // A started or unfinished call is not a successful response.
		}
		if bad {
			failed++
		}
		durations = append(durations, e.Duration)
	}
	state := "unknown"
	if n := len(durations); n > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		median := durations[n/2]
		model.State = "operational"
		if failed > 0 || median >= 10*time.Second {
			model.State = "degraded"
		}
		if failed == n {
			model.State = "unavailable"
		}
		model.Details = fmt.Sprintf("%d of %d recent model calls succeeded. Median call time: %.1fs.", n-failed, n, median.Seconds())
		// Delivery and scheduled execution do not yet have end-to-end monitoring.
		if model.State != "operational" {
			state = "degraded"
		}
	}
	return PublicStatusResponse{State: state, CheckedAt: now.UTC(), Capabilities: []CapabilityHealth{
		{"Website", "operational", "This status page is reachable. Other pages are not independently checked."},
		model,
		{"Mail delivery", "unknown", "Delivery is not currently monitored here."},
		{"Scheduled tasks", "unknown", "Scheduled execution is not currently monitored here."},
	}}
}

func renderPublicStatusHTML(status PublicStatusResponse) string {
	var sb strings.Builder
	title := "Monitoring is partial"
	if status.State == "degraded" {
		title = "Some features are experiencing issues"
	}
	sb.WriteString(Column())
	sb.WriteString(`<div class="page-stack"><section class="page-section"><h2>` + title + `</h2><p class="status-details">Updated ` + html.EscapeString(status.CheckedAt.Format("15:04 UTC")) + `</p></section><section class="page-section">`)
	for _, c := range status.Capabilities {
		label, class := "Not monitored", "status-details"
		switch c.State {
		case "operational":
			label, class = "Operational", "status-ok"
		case "degraded":
			label, class = "Degraded", "status-warn"
		case "unavailable":
			label, class = "Unavailable", "status-error"
		}
		if c.Name == "Assistant models" && c.State == "unknown" {
			label = "No recent data"
		}
		fmt.Fprintf(&sb, `<div class="status-item"><div><span class="status-name">%s</span><p class="status-details">%s</p></div><span class="%s">%s</span></div>`, html.EscapeString(c.Name), html.EscapeString(c.Details), class, label)
	}
	sb.WriteString(`</section><section class="page-section"><h3>Recent reliability</h3><p class="status-details">Model measurements cover calls recorded in the last 15 minutes, within the latest 500 external calls. They include retries and background work; they are not total answer times or an uptime percentage.</p><p class="status-details">Incident history is not yet available. This page runs on the same server as Micro and may be unreachable during an outage.</p></section></div>`)
	sb.WriteString(Close())
	return sb.String()
}
