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
			name:   "at mention with punctuation",
			prompt: "@markets, what is ETH doing today?",
			want:   []string{"markets"},
		},
		{
			name:   "at mention with leading whitespace",
			prompt: "  @markets what is ETH doing today?",
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
			name:   "at mention with leading whitespace",
			prompt: "  @markets what is ETH doing today?",
			want:   "what is ETH doing today?",
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

func TestKeywordRouteRequiresTermBoundaries(t *testing.T) {
	falsePositivePrompts := []string{
		"please postpone the team lunch",
		"this surprise party is busy",
		"watchtower status update",
	}

	for _, prompt := range falsePositivePrompts {
		t.Run(prompt, func(t *testing.T) {
			if got := keywordRoute(prompt); len(got) != 0 {
				t.Fatalf("keywordRoute(%q) = %v, want no keyword route", prompt, got)
			}
		})
	}
}

func TestAllExcludesFallbackAgent(t *testing.T) {
	for _, agent := range All() {
		if agent.ID == "micro" {
			t.Fatal("All() included the micro fallback agent")
		}
	}
}

func TestValidateAgentIDsDeduplicatesAndLimits(t *testing.T) {
	got := validateAgentIDs([]string{"markets", "bogus", "markets", "news", "weather", "mail"})
	want := []string{"markets", "news", "weather"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("validateAgentIDs() = %v, want %v", got, want)
	}
}

func TestValidateAgentIDsFallsBackToMicro(t *testing.T) {
	got := validateAgentIDs([]string{"bogus"})
	want := []string{"micro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("validateAgentIDs() = %v, want %v", got, want)
	}
}

func TestKeywordRouteSingleDomainPriorityIsDeterministic(t *testing.T) {
	prompt := "summarize unread email about the team lunch restaurant"
	want := []string{"mail"}

	for i := 0; i < 100; i++ {
		if got := keywordRoute(prompt); !reflect.DeepEqual(got, want) {
			t.Fatalf("keywordRoute() iteration %d = %v, want %v", i, got, want)
		}
	}
}
