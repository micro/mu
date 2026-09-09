package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPublicStatusUsesRecentOutcomes(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name    string
		entries []*APILogEntry
		want    string
	}{
		{"idle", nil, "unknown"},
		{"old failure", []*APILogEntry{{Kind: "model", Time: now.Add(-time.Hour), Error: "failed"}}, "unknown"},
		{"in flight", []*APILogEntry{{Kind: "model", Time: now, Outcome: "running"}}, "unknown"},
		{"success", []*APILogEntry{{Kind: "model", Time: now, Outcome: "done", Duration: time.Second}}, "operational"},
		{"slow", []*APILogEntry{{Kind: "model", Time: now, Outcome: "done", Duration: 30 * time.Second}}, "degraded"},
		{"failed", []*APILogEntry{{Kind: "model", Time: now, Error: "private provider error"}}, "unavailable"},
		{"mixed", []*APILogEntry{{Kind: "model", Time: now, Error: "failed"}, {Kind: "model", Time: now, Outcome: "done"}}, "degraded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := publicStatusAt(now, tc.entries)
			if s.Capabilities[1].State != tc.want {
				t.Fatalf("got %s, want %s", s.Capabilities[1].State, tc.want)
			}
			if s.State == "operational" {
				t.Fatal("unmonitored capabilities must not produce an all-clear")
			}
		})
	}
}

func TestPublicStatusDoesNotExposeDiagnosticsOrProbeServices(t *testing.T) {
	now := time.Now()
	s := publicStatusAt(now, []*APILogEntry{{Kind: "model", Time: now, Error: "secret-token", URL: "private-url", Model: "private-model", RequestBody: "private-prompt"}})
	b, _ := json.Marshal(s)
	page := renderPublicStatusHTML(s)
	for _, forbidden := range []string{"secret-token", "private-url", "private-model", "private-prompt", "fetch(", "data-path", "All systems operational"} {
		if strings.Contains(string(b)+page, forbidden) {
			t.Errorf("public status contains %q", forbidden)
		}
	}
}
