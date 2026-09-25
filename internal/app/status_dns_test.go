package app

import (
	"strings"
	"testing"
	"time"
)

func TestDNSStatusDistinguishesPendingUnavailableAndMissing(t *testing.T) {
	for _, tc := range []struct {
		name                string
		answer              dnsAnswerState
		state, detail, icon string
	}{
		{"pending", dnsAnswerState{asking: true}, "pending", "checking…", "—"},
		{"unavailable", dnsAnswerState{checked: time.Now(), unavailable: true}, "unavailable", "DNS lookup unavailable", "—"},
		{"missing", dnsAnswerState{checked: time.Now()}, "", "Record not found", "✗"},
		{"published", dnsAnswerState{checked: time.Now(), ok: true}, "", "Record published", "✓"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			question := tc.name + ".example.invalid"
			dnsMu.Lock()
			dnsAnswers[question] = &tc.answer
			dnsMu.Unlock()
			t.Cleanup(func() { dnsMu.Lock(); delete(dnsAnswers, question); dnsMu.Unlock() })
			check := dnsStatus("SPF DNS Record", question, "v=spf1")
			if check.State != tc.state || !strings.Contains(check.Details, tc.detail) {
				t.Fatalf("unexpected check: %+v", check)
			}
			output := renderStatusHTML(StatusResponse{Healthy: true, Config: []StatusCheck{check}})
			row := strings.Split(output, "SPF DNS Record")[1]
			if !strings.Contains(row, ">"+tc.icon+"</span>") {
				t.Fatalf("incorrect icon in %s", row)
			}
			if tc.state != "" && strings.Contains(row, "status-error") {
				t.Fatal("unknown DNS result shown as failure")
			}
		})
	}
}
