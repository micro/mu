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
	model := CapabilityHealth{"AI models", "unknown", "No recent completed model calls."}
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
		if failed > 0 {
			model.State = "degraded"
		}
		if failed == n {
			model.State = "unavailable"
		}
		model.Details = fmt.Sprintf("%d of %d recent model calls succeeded. Median call time: %.1fs.", n-failed, n, median.Seconds())
		state = model.State
	}
	return PublicStatusResponse{State: state, CheckedAt: now.UTC(), Capabilities: []CapabilityHealth{
		{"Website", "operational", "This status page is reachable. Other pages are not independently checked."},
		model,
	}}
}

func renderPublicStatusHTML(status PublicStatusResponse) string {
	var sb strings.Builder
	title := "Micro is reachable"
	guidance := "There are no recent completed agent requests to assess yet."
	switch status.State {
	case "operational":
		title = "Recent agent requests are succeeding"
		guidance = "The website is reachable and recent recorded model calls completed successfully."
	case "degraded":
		title = "Some agent requests are failing"
		guidance = "Replies may be interrupted. Reopen your conversation to check the result before retrying an action."
	case "unavailable":
		title = "Agent requests are failing"
		guidance = "All recently recorded model calls failed. You can still check your saved information; try Micro again later."
	}
	sb.WriteString(Column())
	sb.WriteString(`<div class="page-stack"><section class="page-section"><h2>` + title + `</h2><p>` + html.EscapeString(guidance) + `</p><p class="status-details">Updated ` + html.EscapeString(status.CheckedAt.Format("15:04 UTC")) + `</p></section><section class="page-section">`)
	for _, c := range status.Capabilities {
		label, class := "No recent activity", "status-details"
		switch c.State {
		case "operational":
			label, class = "Operational", "status-ok"
		case "degraded":
			label, class = "Degraded", "status-warn"
		case "unavailable":
			label, class = "Unavailable", "status-error"
		}
		if c.Name == "AI models" && c.State == "unknown" {
			label = "No recent data"
		}
		fmt.Fprintf(&sb, `<div class="status-item"><div><span class="status-name">%s</span><p class="status-details">%s</p></div><span class="%s">%s</span></div>`, html.EscapeString(c.Name), html.EscapeString(c.Details), class, label)
	}
	sb.WriteString(`</section><section class="page-section"><p><a href="/">Micro →</a> · <a href="/contact#support">Report a problem →</a></p><details><summary>About these checks</summary><p class="status-details">Agent results cover recorded model calls in the last 15 minutes, within the latest 500 external calls. They include retries and background work. Call time is not the total time to receive an answer, and a slow successful call is not counted as a failure.</p><p class="status-details">These checks cover this page and recent model calls, not end-to-end mail delivery or scheduled execution. This page runs on the same server as Micro and may be unreachable during an outage.</p></details></section></div>`)
	sb.WriteString(Close())
	return sb.String()
}
