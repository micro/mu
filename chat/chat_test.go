package chat

import (
	"strings"
	"testing"
)

func TestHandlePatternMatchRecognizesKnownPricePromptsWithoutData(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "bitcoin direct price",
			content: "btc price",
			want:    "I don't have current price data for Bitcoin",
		},
		{
			name:    "mention is ignored",
			content: "@micro how much is eth",
			want:    "I don't have current price data for Ethereum",
		},
		{
			name:    "case and whitespace are normalized",
			content: "  PRICE OF GOLD  ",
			want:    "I don't have current price data for Gold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handlePatternMatch(tt.content, nil); got != tt.want {
				t.Fatalf("handlePatternMatch(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestHandlePatternMatchIgnoresUnsupportedPrompts(t *testing.T) {
	tests := []string{
		"",
		"tell me about bitcoin",
		"price",
		"a price",
		"this symbol is too long price",
	}

	for _, content := range tests {
		t.Run(content, func(t *testing.T) {
			if got := handlePatternMatch(content, nil); got != "" {
				t.Fatalf("handlePatternMatch(%q) = %q, want empty string", content, got)
			}
		})
	}
}

func TestGuestChatAuthNoticeExplainsLoginAndAgentFallback(t *testing.T) {
	html := guestChatAuthNotice()

	for _, want := range []string{
		"Sign in to use saved chat.",
		"/agent",
		"Try Mu without an account",
		"/login?redirect=/chat",
		"/signup?redirect=/chat",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat auth notice missing %q in %s", want, html)
		}
	}
}
