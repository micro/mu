package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/service"
)

type CardProbe struct{}

type CardReq struct{ X string }
type CardRsp struct{ Y string }

func (CardProbe) Look(_ context.Context, _ *CardReq, _ *CardRsp) error { return nil }

func registerCardProbes(t *testing.T) {
	t.Helper()
	for _, spec := range []service.Spec{
		{Name: "cardy", Handler: new(CardProbe), Page: "/cardy", Label: "Cardy",
			Icon: "cardy.svg", Card: service.Glance(func() string { return "<b>BODY</b>" })},
		{Name: "cardless", Handler: new(CardProbe), Page: "/cardless", Label: "Cardless",
			Icon: "cardless.svg"},
		{Name: "cardblank", Handler: new(CardProbe), Page: "/cardblank", Label: "Blank",
			Icon: "cardblank.svg", Card: service.Glance(func() string { return "   " })},
		{Name: "cardmine", Handler: new(CardProbe), Page: "/cardmine", Label: "Mine",
			Icon: "cardmine.svg", Scoped: true, Card: service.Glance(func() string { return "<b>PRIVATE</b>" })},
	} {
		if _, already := service.SpecFor(spec.Name); already {
			continue
		}
		if err := service.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
}

// A card is a view of a service, so a tool result renders whatever its service
// shows. markets_list and markets are the same picture; keying cards to tool
// names meant a renamed tool lost its card in silence.
func TestCardForToolResolvesThroughTheService(t *testing.T) {
	registerCardProbes(t)

	got := CardForTool("cardy_list", "")
	if !strings.Contains(got, `class="card"`) || !strings.Contains(got, "Cardy") || !strings.Contains(got, "BODY") {
		t.Errorf("a tool did not resolve to its service's card: %q", got)
	}
	// The bare service name works too — some tools are named for their service.
	if got := CardForTool("cardy", ""); !strings.Contains(got, "BODY") {
		t.Errorf("a bare service name did not resolve: %q", got)
	}
	if got := CardForTool("cardless_list", ""); got != "" {
		t.Errorf("a service with no card rendered %q", got)
	}
	if got := CardForTool("nosuchservice_list", ""); got != "" {
		t.Errorf("an unknown service rendered %q", got)
	}
	// An empty body is not a card, so it gets no wrapper and no heading.
	if got := CardForTool("cardblank_list", ""); got != "" {
		t.Errorf("an empty body was still wrapped: %q", got)
	}
}

func TestCardEndpointServesAFragment(t *testing.T) {
	registerCardProbes(t)

	rec := httptest.NewRecorder()
	CardHandler(rec, httptest.NewRequest("GET", "/card/cardy", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "BODY") || strings.Contains(body, "<html") {
		t.Errorf("expected a bare fragment, got %q", body)
	}

	rec = httptest.NewRecorder()
	CardHandler(rec, httptest.NewRequest("GET", "/card/cardless", nil))
	if rec.Code == 200 {
		t.Error("a service with no card returned a card")
	}
}

// Nothing account-scoped has a card today. If one grew a card it would be one
// person's view served to everybody, from an endpoint that sets a cache header.
func TestScopedServicesNeverServeACard(t *testing.T) {
	registerCardProbes(t)

	rec := httptest.NewRecorder()
	CardHandler(rec, httptest.NewRequest("GET", "/card/cardmine", nil))
	if rec.Code == 200 || strings.Contains(rec.Body.String(), "PRIVATE") {
		t.Errorf("a scoped service served a card: %d %q", rec.Code, rec.Body.String())
	}
}

func TestCardIndexListsWhatHasOne(t *testing.T) {
	registerCardProbes(t)

	rec := httptest.NewRecorder()
	CardHandler(rec, httptest.NewRequest("GET", "/card", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `"/card/cardy"`) {
		t.Errorf("the index does not list a service with a card: %s", body)
	}
	if strings.Contains(body, "cardless") {
		t.Errorf("the index lists a service with no card: %s", body)
	}
}
