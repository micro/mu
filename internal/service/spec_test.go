package service

import (
	"context"
	"testing"
)

type SpecProbe struct{}

type SpecReq struct{ X string }
type SpecRsp struct{ Y string }

func (SpecProbe) Read(_ context.Context, _ *SpecReq, _ *SpecRsp) error  { return nil }
func (SpecProbe) Erase(_ context.Context, _ *SpecReq, _ *SpecRsp) error { return nil }

// Everything a surface used to keep its own list of now comes off the Spec.
// Before this there were separate maps for account scoping, destructive
// methods and nav labels, each maintained by hand in a different package.
func TestSurfacesDeriveFromTheSpec(t *testing.T) {
	err := Register(Spec{
		Name:        "specprobe",
		Handler:     new(SpecProbe),
		Description: "A probe",
		Page:        "/probe",
		Label:       "Probe Things",
		Scoped:      true,
		Endpoints: map[string]Endpoint{
			"Read":  {Doc: "Read a thing", Cost: "probe_read"},
			"Erase": {Doc: "Erase a thing", Destructive: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !AccountScoped("specprobe") {
		t.Error("AccountScoped did not derive Scoped")
	}
	if !Destructive("specprobe", "Erase") {
		t.Error("Destructive did not derive the endpoint flag")
	}
	if Destructive("specprobe", "Read") {
		t.Error("a read was reported destructive")
	}
	if got := CostOf("specprobe", "Read"); got != "probe_read" {
		t.Errorf("CostOf = %q, want probe_read", got)
	}
	if got := CostOf("specprobe", "Erase"); got != "" {
		t.Errorf("free endpoint reported cost %q", got)
	}
	if got := Label("specprobe"); got != "Probe Things" {
		t.Errorf("Label = %q, want the declared label", got)
	}
	// The description still has to reach the registry, which is what the agent
	// reads when choosing a tool.
	if got := EndpointDescriptions("specprobe")["SpecProbe.Read"]; got != "Read a thing" {
		t.Errorf("registry description = %q, want %q", got, "Read a thing")
	}
}

// An unknown service must not be reported scoped or destructive — but it also
// has no tools, because tools come from the registry.
func TestUnknownServiceDerivesNothing(t *testing.T) {
	if AccountScoped("nope") || Destructive("nope", "Anything") || CostOf("nope", "X") != "" {
		t.Error("an unregistered service derived a policy")
	}
	if got := Label("nope"); got != "Nope" {
		t.Errorf("Label fallback = %q, want title-cased name", got)
	}
}

// The tool name is derived, never written.
func TestToolNameIsDerived(t *testing.T) {
	s := Spec{Name: "web"}
	if got := s.Tool("Search"); got != "web_search" {
		t.Errorf("Tool = %q, want web_search", got)
	}
	if got := (Spec{Name: "db"}).Tool("Create"); got != "db_create" {
		t.Errorf("Tool = %q, want db_create", got)
	}
}

// A headless service is one with no page.
func TestHeadlessIsDerivedFromPage(t *testing.T) {
	if !(Spec{Name: "index"}).Headless() {
		t.Error("a service with no page should be headless")
	}
	if (Spec{Name: "news", Page: "/news"}).Headless() {
		t.Error("a service with a page is not headless")
	}
}

// Register must not silently accept a spec it cannot stand up.
func TestRegisterRejectsAnIncompleteSpec(t *testing.T) {
	if err := Register(Spec{Handler: new(SpecProbe)}); err == nil {
		t.Error("a spec with no name was accepted")
	}
	if err := Register(Spec{Name: "nohandler"}); err == nil {
		t.Error("a spec with no handler was accepted")
	}
}
