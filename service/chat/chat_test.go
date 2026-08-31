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

// The way out of this notice has to be a way in.
//
// The previous version of this test asserted the notice contained "/agent" and
// "Try Mu without an account", both of which it did, and the link bounced every
// reader who took it to /login — /agent checks auth in its handler. The test
// pinned the copy and was silent about the one thing the copy promised.
//
// So: name the sign-in doors, and require the no-account door to be the front
// page, which answers a stranger. TestNoSignedOutCTASendsAStrangerToLogin in
// test/ holds the general rule against the route table.
func TestGuestChatAuthNoticeOffersADoorAStrangerCanOpen(t *testing.T) {
	html := guestChatAuthNotice()

	for _, want := range []string{
		"Sign in to use saved chat.",
		`href="/"`,
		"/login?redirect=/chat",
		"/signup?redirect=/chat",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat auth notice missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, `href="/agent"`) {
		t.Error("the notice sends a signed-out reader to /agent, which refuses " +
			"without a session — the offer of a way in is a redirect to /login")
	}
}

func TestCurrentSummaryMetaDefaultsUnavailable(t *testing.T) {
	oldMeta := summaryMeta
	summaryMeta = SummaryMetadata{}
	defer func() { summaryMeta = oldMeta }()

	meta := currentSummaryMeta()
	if meta.Status != "unavailable" {
		t.Fatalf("Status = %q, want unavailable", meta.Status)
	}
	if meta.Source == "" {
		t.Fatalf("Source should explain summary provenance")
	}
}
