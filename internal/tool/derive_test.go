package tool

import (
	"context"
	"strings"
	"testing"

	"mu/internal/api"

	"mu/internal/service"
)

type DeriveProbe struct{}

type ThingRequest struct {
	Query string `json:"query" description:"What to look for"`
	Limit int    `json:"limit" description:"How many"`
	Deep  bool   `json:"deep" description:"Go deeper"`
	hide  string //nolint:unused // unexported: must not become a parameter
}

type ThingResponse struct {
	Text string `json:"text"`
}

func (DeriveProbe) Find(_ context.Context, _ *ThingRequest, rsp *ThingResponse) error {
	rsp.Text = "found"
	return nil
}

// An endpoint declared on a Spec becomes a tool without anyone writing it out a
// second time. Six had drifted out of reach before this existed — none of them
// withheld on purpose, just never typed twice.
func TestEveryEndpointBecomesATool(t *testing.T) {
	if _, already := service.SpecFor("derivable"); !already {
		if err := service.Register(service.Spec{
			Name: "derivable", Handler: new(DeriveProbe), Page: "/derivable",
			Label: "Derivable", Icon: "derivable.svg",
			Endpoints: map[string]service.Endpoint{
				"Find": {Doc: "Find a thing", Cost: "web_search"},
			},
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	DeriveTools()

	var got *api.Tool
	for _, reg := range api.Tools() {
		if reg.Name == "derivable_find" {
			found := reg
			got = &found
		}
	}
	if got == nil {
		t.Fatal("an endpoint on a Spec produced no tool")
	}
	if got.Description != "Find a thing" {
		t.Errorf("the endpoint's own doc was not used: %q", got.Description)
	}
	// The reason this stayed open: a derived tool with no operation would be a
	// free path to a paid service. The Endpoint already declares its cost.
	if got.WalletOp != "web_search" {
		t.Errorf("a derived tool lost its price: %q", got.WalletOp)
	}

	names := map[string]string{}
	for _, p := range got.Params {
		names[p.Name] = p.Type
	}
	for name, want := range map[string]string{"query": "string", "limit": "number", "deep": "boolean"} {
		if names[name] != want {
			t.Errorf("parameter %s typed %q, want %q", name, names[name], want)
		}
	}
	if len(names) != 3 {
		t.Errorf("an unexported field became a parameter: %v", names)
	}
}

// A hand-written registration always wins: those carry descriptions and
// parameter docs written for a model, and often return one field of a response
// rather than the whole struct. Derivation is for what nobody remembered.
func TestAWrittenToolIsNotOverwritten(t *testing.T) {
	api.RegisterTool(api.Tool{Name: "written_thing", Description: "By hand"})
	if _, already := service.SpecFor("written"); !already {
		if err := service.Register(service.Spec{
			Name: "written", Handler: new(DeriveProbe), Page: "/written",
			Label: "Written", Icon: "written.svg",
			Endpoints: map[string]service.Endpoint{"Thing": {Doc: "Derived"}},
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	DeriveTools()

	seen := 0
	for _, reg := range api.Tools() {
		if reg.Name == "written_thing" {
			seen++
			if reg.Description != "By hand" {
				t.Errorf("derivation overwrote a written tool: %q", reg.Description)
			}
		}
	}
	if seen != 1 {
		t.Errorf("derivation registered %d copies of an existing tool", seen)
	}
}

// Running it twice must not double the registry — main() calls it once, but a
// second call is exactly the mistake that would go unnoticed.
func TestDerivingTwiceChangesNothing(t *testing.T) {
	DeriveTools()
	before := len(api.Tools())
	DeriveTools()
	if after := len(api.Tools()); after != before {
		t.Errorf("a second derivation added %d tools", after-before)
	}
}

// The response these endpoints are written to return is model-ready prose, not
// a struct to unpack.
func TestASingleTextFieldComesBackAsText(t *testing.T) {
	if got := renderResponse(map[string]any{"events": "No upcoming events."}); got != "No upcoming events." {
		t.Errorf("a lone string field was not returned as text: %q", got)
	}
	if got := renderResponse(map[string]any{"text": "hello", "count": 2.0}); got != "hello" {
		t.Errorf("a text field was not preferred: %q", got)
	}
	got := renderResponse(map[string]any{"rooms": []any{}, "n": 1.0})
	if !strings.HasPrefix(got, "{") {
		t.Errorf("a structured response should come back as JSON: %q", got)
	}
}
