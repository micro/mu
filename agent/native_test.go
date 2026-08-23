package agent

import (
	"testing"

	gmai "go-micro.dev/v6/ai"
)

func TestNativeToolCallKeyDedupesEquivalentInputs(t *testing.T) {
	first := nativeToolCallKey(gmai.ToolCall{Name: "markets_Get", Input: map[string]any{"category": "crypto", "limit": float64(10)}})
	second := nativeToolCallKey(gmai.ToolCall{Name: "markets_Get", Input: map[string]any{"limit": float64(10), "category": "crypto"}})
	if first != second {
		t.Fatalf("expected equivalent native tool inputs to share a dedupe key: %q vs %q", first, second)
	}
	if first == nativeToolCallKey(gmai.ToolCall{Name: "markets_Get", Input: map[string]any{"category": "commodities", "limit": float64(10)}}) {
		t.Fatal("expected distinct native market inputs to keep distinct dedupe keys")
	}
}

// A run record has to name the tool the way everything else names it.
//
// go-micro reports its handler name — "context_Server_Get" — and the display
// label is "⚙️ Working", which is right for somebody watching and worthless as
// a trace. /runs showed the label for a while, so a finished run said the agent
// had done some working. A trace naming something no other page names is a
// trace you cannot look up.
func TestNativeToolNameMatchesTheToolCallersUse(t *testing.T) {
	for raw, want := range map[string]string{
		"context_Server_Get":      "context_get",
		"memory_Server_Set":       "memory_set",
		"news_Server_List":        "news_list",
		"weather.Server.Forecast": "weather_forecast",
		"db_Handler_Create":       "db_create",
		"already_fine":            "already_fine",
	} {
		if got := NativeToolName(raw); got != want {
			t.Errorf("NativeToolName(%q) = %q, want %q", raw, got, want)
		}
	}
}

// A scope that matches nothing leaves an agent with nothing.
//
// filterServices returned the whole set when the intersection was empty,
// commented "never leave an agent with no tools" — a convenience overriding a
// boundary. It is reachable: nativeServices(public) is a smaller set in a
// shared room, so an agent scoped to mail and contacts there fell past its own
// scope to everything public.
//
// Failing open on a permission check is the one place the helpful answer is the
// wrong one.
func TestAScopeThatMatchesNothingGrantsNothing(t *testing.T) {
	all := []string{"news", "markets", "weather"}

	if got := filterServices(all, []string{"mail", "contacts"}); len(got) != 0 {
		t.Errorf("an agent scoped to services that are not here got %v", got)
	}

	// An unscoped agent still gets everything — empty means "no restriction",
	// which is a different statement from "restricted to nothing".
	if got := filterServices(all, nil); len(got) != len(all) {
		t.Errorf("an unscoped agent was restricted to %v", got)
	}

	// And a scope that matches some of them gets exactly those.
	got := filterServices(all, []string{"news", "mail"})
	if len(got) != 1 || got[0] != "news" {
		t.Errorf("the intersection is %v, want just news", got)
	}
}
