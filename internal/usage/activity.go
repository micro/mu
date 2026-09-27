package usage

import (
	"strings"
	"time"
)

// Activity contains operational metadata only, never arguments, bodies or secrets.
type Activity struct {
	At         time.Time `json:"at"`
	Surface    string    `json:"surface"`
	Operation  string    `json:"operation"`
	Account    string    `json:"account"`
	TokenID    string    `json:"token_id,omitempty"`
	Outcome    string    `json:"outcome"`
	Status     int       `json:"status,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

const maxActivity = 5000
const maxFailures = 2000

func RecordActivity(a Activity) {
	if a.At.IsZero() {
		a.At = now().UTC()
	}
	if a.Account == "" {
		a.Account = "guest"
	}
	if len(a.Operation) > 160 {
		a.Operation = a.Operation[:160]
	}
	mu.Lock()
	defer mu.Unlock()
	rings.Activity = append(rings.Activity, a)
	if len(rings.Activity) > maxActivity {
		rings.Activity = rings.Activity[len(rings.Activity)-maxActivity:]
	}
	if !routineNotFound(a) && a.Outcome != "ok" && a.Outcome != "created" && a.Outcome != "revoked" {
		rings.Failures = append(rings.Failures, a)
		if len(rings.Failures) > maxFailures {
			rings.Failures = rings.Failures[len(rings.Failures)-maxFailures:]
		}
	}
	dirty = true
}
func Activities(failures bool) []Activity {
	mu.Lock()
	defer mu.Unlock()
	src := rings.Activity
	if failures {
		src = rings.Failures
	}
	out := []Activity{}
	cutoff := now().AddDate(0, 0, -14)
	for i := len(src) - 1; i >= 0; i-- {
		if src[i].At.After(cutoff) && (!failures || !routineNotFound(src[i])) {
			out = append(out, src[i])
		}
	}
	return out
}

// Classify a failure without retaining potentially sensitive error text.
func FailureKind(status int, message string) string {
	s := strings.ToLower(message)
	switch {
	case strings.Contains(s, "credit"), strings.Contains(s, "balance"), strings.Contains(s, "quota"), status == 402:
		return "credits or quota"
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline"), status == 408, status == 504:
		return "timeout"
	case status == 429, strings.Contains(s, "rate limit"):
		return "rate limited"
	case status == 404:
		return "not found"
	case status == 400:
		return "invalid request"
	case status == 405:
		return "method not allowed"
	case status == 401:
		return "authentication refused"
	case status == 403, strings.Contains(s, "forbidden"), strings.Contains(s, "permission"), strings.Contains(s, "blocked"), strings.Contains(s, "authorization"):
		return "access blocked"
	case status >= 500:
		return "service failure"
	default:
		return "request failed"
	}
}

// Anonymous missing-page requests remain in Activity, not the actionable feed.
// This does not infer bot activity or hide signed-in users' broken links.
func routineNotFound(a Activity) bool {
	return a.Surface == "http" && a.Status == 404 && (a.Account == "" || a.Account == "guest")
}
