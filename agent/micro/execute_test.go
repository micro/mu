package micro

import (
	"strings"
	"testing"

	"mu/tool"

	"mu/internal/service"
	"mu/service/index"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/weather"
	"mu/service/web"
)

func TestBuildToolsDescFiltersPrivateToolsForGuests(t *testing.T) {
	// The tools come from the Specs, and the catalogue is what turns one into
	// the other. Registering the services is not enough on its own — without
	// this the list is empty and "guests cannot see mail_inbox" passes because
	// nobody can.
	registerServices(t)
	tool.DeriveTools()

	mail := Get("mail")
	if mail == nil {
		t.Fatal("mail agent is not registered")
	}

	privateTools := mail.buildToolsDesc(false)
	if !strings.Contains(privateTools, "mail_inbox") {
		t.Fatalf("private mail tools should include mail_read, got %q", privateTools)
	}

	guestTools := mail.buildToolsDesc(true)
	if strings.Contains(guestTools, "mail_inbox") || strings.Contains(guestTools, "mail_send") {
		t.Fatalf("guest mail tools should exclude private mail tools, got %q", guestTools)
	}
}

// The guest rule is derived from each service's Spec, so the services have to
// be registered for it to answer.
func registerServices(t *testing.T) {
	t.Helper()
	for _, s := range []service.Spec{
		news.Spec, markets.Spec, weather.Spec, web.Spec, index.Spec, mail.Spec,
	} {
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
}

func TestGuestAllowedToolsCoverPublicCoreServices(t *testing.T) {
	registerServices(t)
	for _, tool := range []string{"weather_forecast", "news_list", "markets_list", "web_search", "index_search"} {
		t.Run(tool, func(t *testing.T) {
			if !isGuestAllowedTool(tool) {
				t.Fatalf("%s should be allowed for guest ask-answer smoke paths", tool)
			}
		})
	}

	for _, tool := range []string{"mail_inbox", "mail_send"} {
		t.Run(tool, func(t *testing.T) {
			if isGuestAllowedTool(tool) {
				t.Fatalf("%s should stay private for guest ask-answer smoke paths", tool)
			}
		})
	}
}
