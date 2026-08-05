package api

import (
	"context"
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

	svc := serviceGrid()
	if !strings.Contains(svc, `href="/catpaged"`) {
		t.Errorf("the services lens is missing a paged service:\n%s", svc)
	}
	if !strings.Contains(svc, ">Cat Paged<") || !strings.Contains(svc, "A service with a page") {
		t.Error("a service tile dropped its declared label or description")
	}
	// Headless services still appear. One with no page is still something this
	// instance runs, and leaving it out would understate the answer to "is any
	// of this real" — which is what the catalogue exists to answer.
	if !strings.Contains(svc, ">Cat Hidden<") || !strings.Contains(svc, "agents and apps only") {
		t.Error("the services lens hid the headless services")
	}

	if !strings.Contains(lensTabs(true), `class="lens-tab active" href="/services"`) {
		t.Error("the services lens does not mark itself active")
	}
	if !strings.Contains(lensTabs(false), `class="lens-tab active" href="/tools"`) {
		t.Error("the tools lens does not mark itself active")
	}
}
