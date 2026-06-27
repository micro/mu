package micro

import (
	"reflect"
	"testing"
)

func TestRouteDirectAddressAvoidsLLM(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   []string
	}{
		{
			name:   "at mention",
			prompt: "@markets what is ETH doing today?",
			want:   []string{"markets"},
		},
		{
			name:   "ask the agent",
			prompt: "ask the weather agent about Lisbon tomorrow",
			want:   []string{"weather"},
		},
		{
			name:   "use agent",
			prompt: "use mail to summarize unread messages",
			want:   []string{"mail"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Route(tt.prompt); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Route(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestStripAddress(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name:   "at mention",
			prompt: "@markets what is ETH doing today?",
			want:   "what is ETH doing today?",
		},
		{
			name:   "ask agent about",
			prompt: "ask the weather agent about Lisbon tomorrow",
			want:   "Lisbon tomorrow",
		},
		{
			name:   "use agent",
			prompt: "use mail summarize unread messages",
			want:   "summarize unread messages",
		},
		{
			name:   "unaddressed prompt",
			prompt: "summarize unread messages",
			want:   "summarize unread messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripAddress(tt.prompt); got != tt.want {
				t.Fatalf("StripAddress(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestKeywordRouteMultiSignalOrdering(t *testing.T) {
	got := keywordRoute("give me weather, BTC price, news headlines, and youtube videos")
	want := []string{"weather", "news", "markets"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keywordRoute() = %v, want %v", got, want)
	}
}

func TestKeywordRouteTradeTakesPrecedenceOverMarkets(t *testing.T) {
	got := keywordRoute("should I buy BTC after this price move?")
	want := []string{"trade"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keywordRoute() = %v, want %v", got, want)
	}
}

func TestAllExcludesFallbackAgent(t *testing.T) {
	for _, agent := range All() {
		if agent.ID == "micro" {
			t.Fatal("All() included the micro fallback agent")
		}
	}
}
