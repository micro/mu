package blog

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// TestContributorsComeFromTheCatalogue is the whole point: add a service and
// the digest can talk about it without anybody editing this package.
func TestContributorsComeFromTheCatalogue(t *testing.T) {
	registered := map[string]bool{}
	for _, s := range service.Specs() {
		registered[s.Name] = true
	}
	if len(registered) == 0 {
		t.Skip("nothing registered yet; TestANewServiceReachesTheDigest registers one")
	}

	for _, c := range contributors("tech") {
		if !registered[c.service] {
			t.Errorf("asked %s, which is not a registered service", c.service)
		}
	}
}

// TestNothingScopedOrPaidIsAsked — the digest is written by the instance about
// the world. One person's mail is not the world, and a paid call is not
// something to make thirty of on a schedule.
func TestNothingScopedOrPaidIsAsked(t *testing.T) {
	for _, c := range contributors("") {
		spec, ok := service.SpecFor(c.service)
		if !ok {
			t.Errorf("%s is not registered", c.service)
			continue
		}
		if spec.Scoped {
			t.Errorf("%s is account-scoped and was asked to contribute", c.service)
		}
		ep := spec.Endpoints[c.endpoint]
		if ep.Cost != "" {
			t.Errorf("%s.%s costs %s and was asked on a schedule", c.service, c.endpoint, ep.Cost)
		}
		if ep.Destructive {
			t.Errorf("%s.%s is destructive and was asked for material", c.service, c.endpoint)
		}
	}
}

func TestProsePicksTheReadablePart(t *testing.T) {
	if got := prose(map[string]any{"text": "  headlines  "}); got != "headlines" {
		t.Errorf("prose(text) = %q", got)
	}
	if got := prose(map[string]any{"reminder": "a verse and a saying, at some length"}); got == "" {
		t.Error("a reminder was not read as prose")
	}
	// An id or a status word is not material, and printing a Go map into a
	// prompt is worse than contributing nothing.
	if got := prose(map[string]any{"id": "abc123"}); got != "" {
		t.Errorf("prose(id) = %q, want nothing", got)
	}
	if got := prose(map[string]any{}); got != "" {
		t.Errorf("prose(empty) = %q", got)
	}
}

func TestHeadingsReadLikeSections(t *testing.T) {
	spec := service.Spec{Name: "flights", Label: "Flights"}
	if got := headingFor(spec, "List"); got != "Flights" {
		t.Errorf("headingFor(List) = %q", got)
	}
	if got := headingFor(service.Spec{Name: "weather"}, "Forecast"); !strings.Contains(got, "Weather") {
		t.Errorf("an unlabelled service got heading %q", got)
	}
}

// SkywatchServer is a service that did not exist when this file was written, which
// is exactly the point being tested.
type SkywatchServer struct{}

type SkywatchListRequest struct{}
type SkywatchListResponse struct {
	Text string `json:"text"`
}

func (SkywatchServer) List(_ context.Context, _ *SkywatchListRequest, rsp *SkywatchListResponse) error {
	rsp.Text = "Two aircraft overhead, both descending into Heathrow."
	return nil
}

// TestANewServiceReachesTheDigestWithNoCodeChange is the claim the whole
// rewrite was for.
//
// Flights shipped and the daily opinion could not mention it, because gathering
// named four packages in code. Registering a service the digest has never heard
// of must be enough — no edit here, no import, no list to append to.
func TestANewServiceReachesTheDigestWithNoCodeChange(t *testing.T) {
	spec := service.Spec{
		Name:        "skywatch",
		Handler:     new(SkywatchServer),
		Description: "a service invented by this test",
		Label:       "Skywatch",
		Endpoints: map[string]service.Endpoint{
			"List": {Doc: "What is overhead"},
		},
	}
	if _, already := service.SpecFor(spec.Name); !already {
		if err := service.Register(spec); err != nil {
			t.Fatalf("registering a service: %v", err)
		}
	}

	found := false
	for _, c := range contributors("tech") {
		if c.service == "skywatch" {
			found = true
		}
	}
	if !found {
		t.Fatal("a newly registered service was not offered the chance to contribute")
	}

	material := gatherFromCatalogue("tech")
	if !strings.Contains(material, "Skywatch") {
		t.Errorf("the new service is missing its heading:\n%s", material)
	}
	if !strings.Contains(material, "descending into Heathrow") {
		t.Errorf("the new service contributed no material:\n%s", material)
	}
}
