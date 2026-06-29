package micro

import (
	"strings"
	"testing"
)

func TestBuildToolsDescFiltersPrivateToolsForGuests(t *testing.T) {
	mail := Get("mail")
	if mail == nil {
		t.Fatal("mail agent is not registered")
	}

	privateTools := mail.buildToolsDesc(false)
	if !strings.Contains(privateTools, "mail_read") {
		t.Fatalf("private mail tools should include mail_read, got %q", privateTools)
	}

	guestTools := mail.buildToolsDesc(true)
	if strings.Contains(guestTools, "mail_read") || strings.Contains(guestTools, "mail_send") {
		t.Fatalf("guest mail tools should exclude private mail tools, got %q", guestTools)
	}
}

func TestGuestAllowedToolsCoverPublicCoreServices(t *testing.T) {
	for _, tool := range []string{"weather_forecast", "news_headlines", "markets", "web_search", "search"} {
		t.Run(tool, func(t *testing.T) {
			if !isGuestAllowedTool(tool) {
				t.Fatalf("%s should be allowed for guest ask-answer smoke paths", tool)
			}
		})
	}

	for _, tool := range []string{"mail_read", "mail_send"} {
		t.Run(tool, func(t *testing.T) {
			if isGuestAllowedTool(tool) {
				t.Fatalf("%s should stay private for guest ask-answer smoke paths", tool)
			}
		})
	}
}
