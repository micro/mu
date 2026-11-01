package admin

import (
	"mu/internal/usage"
	"strings"
	"testing"
	"time"
)

func TestHostUsageSeparatesPaymentsAndCalls(t *testing.T) {
	now := time.Now()
	payment := usage.Activity{At: now, Host: "tools.example", Surface: "payment", Operation: "search", Account: "payer", PaymentMethod: "x402", Outcome: "ok", AmountAtomic: "12345", Asset: "USDC", Network: "base", Transaction: "tx1"}
	entries := []usage.Activity{payment, payment,
		{At: now, Host: "tools.example", Surface: "mcp", Operation: "search", Account: "payer", PaymentMethod: "wallet", Outcome: "ok"},
		{At: now, Host: "tools.example", Surface: "http", Path: "/mcp", Operation: "POST /mcp", Status: 402},
		{At: now, Host: "<other>", Surface: "api", Operation: "read", Account: "alice", PaymentMethod: "account", Outcome: "ok"},
		{At: now, Host: "<other>", Surface: "api", Operation: "read", Account: "alice", PaymentMethod: "account", Outcome: "ok"},
		{At: now.Add(-time.Hour * 48), Host: "expired", Surface: "mcp", Outcome: "ok"},
	}
	got := hostUsageCard(entries, now.Add(-time.Hour), "")
	for _, want := range []string{"12345 atomic units", "x402 settlements", "wallet tool calls", "HTTP requests", "&lt;other&gt;", "<td>2</td><td>2</td><td>1</td><td>1</td>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, "24690") || strings.Contains(got, "expired") || strings.Contains(got, "<other>") {
		t.Fatal(got)
	}
	filtered := hostUsageCard(entries, now.Add(-time.Hour), "alice")
	if strings.Contains(filtered, "tools.example") {
		t.Fatal("caller filter ignored", filtered)
	}
}
