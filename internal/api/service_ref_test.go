package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/quota"
	"mu/internal/service"
)

type RefProbe struct{}

type RefReq struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}
type RefRsp struct{ Text string }

func (RefProbe) List(_ context.Context, _ *RefReq, _ *RefRsp) error   { return nil }
func (RefProbe) Delete(_ context.Context, _ *RefReq, _ *RefRsp) error { return nil }

var refSpec = service.Spec{
	Name: "refprobe", Handler: new(RefProbe), Page: "/refprobe",
	Description: "A service to render a reference for", Label: "Ref Probe", Icon: "refprobe.svg",
	Endpoints: map[string]service.Endpoint{
		"List":   {Doc: "Read the things"},
		"Delete": {Doc: "Remove a thing", Destructive: true, Cost: quota.OpWebSearch},
	},
}

func refFixture(t *testing.T) {
	t.Helper()
	if _, already := service.SpecFor(refSpec.Name); !already {
		if err := service.Register(refSpec); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	// The reference lists a method only when a tool came out of it — see
	// restMethods. The registry is filled from tool/ in the real binary and by
	// hand here.
	for _, tl := range []Tool{
		{Name: "refprobe_list", Description: "Read the things",
			Params: []ToolParam{{Name: "query", Type: "string", Description: "What to look for", Required: true}}},
		{Name: "refprobe_delete", Description: "Remove a thing", WalletOp: quota.OpWebSearch,
			Params: []ToolParam{{Name: "query", Type: "string", Description: "Which one", Required: true}}},
	} {
		if _, ok := ToolByName(tl.Name); !ok {
			tl.Handle = func(map[string]any) (string, error) { return "ok", nil }
			RegisterTool(tl)
		}
	}
}

// The page is the Spec, rendered. Nothing on it is typed out per service, so
// the assertion is that each declared thing arrives: the method, its argument,
// its price, and the verb that follows from whether it changes anything.
func TestAServiceReferenceIsDerivedFromItsSpec(t *testing.T) {
	refFixture(t)
	out := serviceRef(refSpec, service.Anyone(), "https://example.test")

	for _, want := range []string{
		"GET /api/v1/refprobe/list",    // a read
		"POST /api/v1/refprobe/delete", // and one that changes something
		"Read the things",              // the doc off the Endpoint
		"query",                        // the declared argument
		`href="/refprobe"`,             // the way out to the service itself
		"A service to render a reference for",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the reference does not carry %q", want)
		}
	}
}

// A method that costs money says so before the button is pressed. Nothing else
// in the product tells you the price of a thing at the moment you do it.
func TestAPricedMethodShowsItsPriceOnTheForm(t *testing.T) {
	refFixture(t)
	out := serviceRef(refSpec, service.Anyone(), "https://example.test")
	if !strings.Contains(out, "Costs ") {
		t.Errorf("a priced method's form does not say what it costs:\n%s", out)
	}
}

// The form makes the real call. A playground that answered from somewhere else
// would be a fourth place for the truth about a call to live, and the first to
// disagree with the other three.
func TestTheFormCallsTheRealDoor(t *testing.T) {
	refFixture(t)
	out := serviceRef(refSpec, service.Anyone(), "https://example.test")

	if !strings.Contains(out, `data-path="/api/v1/refprobe/list" data-method="GET"`) {
		t.Error("a read's form does not GET the API door")
	}
	if !strings.Contains(out, `data-path="/api/v1/refprobe/delete" data-method="POST"`) {
		t.Error("a destructive method's form does not POST the API door")
	}
}

// A service whose page is this page does not offer a button to itself. That is
// weather and hazards: their page was derived, the derived page turned out to
// be this one, and /weather redirects here.
func TestAServiceWhosePageIsItsReferenceOffersNoWayOut(t *testing.T) {
	refFixture(t)

	own := refSpec
	own.Page = "/services/" + own.Name
	out := serviceRef(own, service.Anyone(), "https://example.test")

	if strings.Contains(out, `class="svc-open"`) {
		t.Error("a service whose page is this page offers a button to itself")
	}
	if !strings.Contains(out, "GET /api/v1/refprobe/list") {
		t.Error("it lost the methods")
	}
}

// An unknown name is a 404, not a blank page with a heading on it.
func TestAnUnknownServiceIsNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	ServiceRefHandler(w, httptest.NewRequest("GET", "/services/nosuchthing", nil))
	if w.Code != 404 {
		t.Errorf("/services/nosuchthing answered %d, want 404", w.Code)
	}
}

// One segment. /services/news/anything is not a page, and answering it with the
// news reference would publish a URL that means nothing.
func TestADeeperPathIsNotTheService(t *testing.T) {
	refFixture(t)
	w := httptest.NewRecorder()
	ServiceRefHandler(w, httptest.NewRequest("GET", "/services/refprobe/extra", nil))
	if w.Code != 404 {
		t.Errorf("/services/refprobe/extra answered %d, want 404", w.Code)
	}
}
