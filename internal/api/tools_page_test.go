package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/service"
)

type CatProbe struct{}

type CatReq struct{ X string }
type CatRsp struct{ Y string }

func (CatProbe) Look(_ context.Context, _ *CatReq, _ *CatRsp) error { return nil }

// One catalogue, two lenses. The person's lens lists services, the agent's
// lists tools, and both derive from the same Specs so the two cannot drift —
// which is why they are one page and not two.
func TestTheCatalogueHasBothLenses(t *testing.T) {
	for _, spec := range []service.Spec{
		{Name: "catpaged", Handler: new(CatProbe), Page: "/catpaged",
			Description: "A service with a page", Label: "Cat Paged", Icon: "catpaged.svg"},
		{Name: "cathidden", Handler: new(CatProbe),
			Description: "A service with no page", Label: "Cat Hidden", Icon: "cathidden.svg"},
	} {
		if _, already := service.SpecFor(spec.Name); already {
			continue
		}
		if err := service.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}

	svc := serviceGrid(httptest.NewRequest("GET", "/services", nil))

	// Every tile leads to the service's reference, not to the service's own
	// page. A reader on this page is asking what a thing is and how to call it;
	// the thing itself is one button away at the top of the reference.
	if !strings.Contains(svc, `href="/services/catpaged"`) {
		t.Errorf("the services lens is missing a paged service:\n%s", svc)
	}
	if !strings.Contains(svc, ">Cat Paged<") || !strings.Contains(svc, "A service with a page") {
		t.Error("a service tile dropped its declared label or description")
	}

	// A headless service is listed. It used to be left out, on the reading that
	// this lens promises "what you can open" — which meant the page called
	// Services under-reported what the instance runs, and hid three real
	// services behind a property of their UI. It was then listed as a dead
	// tile, which was better and still not clickable.
	//
	// Every service has a reference page, so there is no such thing as a
	// service with nowhere to go any more.
	if !strings.Contains(svc, `href="/services/cathidden"`) {
		t.Errorf("a headless service is not reachable from the services lens:\n%s", svc)
	}
	if !strings.Contains(svc, ">Cat Hidden<") {
		t.Errorf("a headless service is missing from the services lens:\n%s", svc)
	}
	if strings.Contains(svc, `href=""`) {
		t.Error("a service was rendered as a link to nowhere")
	}

	// A tile has three targets: what it is, the thing itself, and its tools.
	// The reference answers "what is this and how do I call it", which is what
	// a reader on this page is asking — and somebody who already knows should
	// not have to read it again to reach the service. A second target on the
	// tile rather than a second grid on the page.
	if !strings.Contains(svc, `class="service-tile-open" href="/catpaged"`) {
		t.Errorf("a paged service has no way from its tile to its own page:\n%s", svc)
	}
	// A headless service has nothing to open, so it offers nothing.
	if strings.Contains(svc, `class="service-tile-open" href="/cathidden"`) {
		t.Error("a headless service offers a way to a page it does not have")
	}
	// And a service whose page is its reference — weather, hazards — must not
	// offer a way to where the tile already goes.
	if strings.Contains(svc, `class="service-tile-open" href="/services/`) {
		t.Error("a tile offers Open pointing back at the reference it already links to")
	}

	// The two lenses are reached from the sidebar, so neither renders a switch
	// of its own — the tools lens lists tools and nothing else.
	if strings.Contains(toolGrid(), "service-tile") {
		t.Error("the tools lens rendered service tiles")
	}
}
