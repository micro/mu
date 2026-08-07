package service

// The sidebar shows what a reader pinned.
//
// It showed nineteen services in alphabetical order, which put Wallet
// eighteenth between Video and Weather, and then it showed none of them —
// because the three levels are what the product is and a list of nineteen
// buried them. That was right for arriving and wrong for using: reaching for a
// service you use every day meant opening the catalogue and hunting.
//
// A chosen list is the resolution. It is short because you chose it, ordered
// because you ordered it, and empty until you pin something — so a developer
// arriving at the three levels still sees exactly those.

import (
	"context"
	"testing"
)

type PinProbeReq struct{}
type PinProbeRsp struct {
	OK bool `json:"ok"`
}
type PinProbeHandler struct{}

func (PinProbeHandler) Ping(_ context.Context, _ *PinProbeReq, rsp *PinProbeRsp) error {
	rsp.OK = true
	return nil
}

func registerPinProbes(t *testing.T) {
	t.Helper()
	for _, s := range []Spec{
		{Name: "pinalpha", Handler: PinProbeHandler{}, Page: "/pinalpha"},
		{Name: "pinbeta", Handler: PinProbeHandler{}, Page: "/pinbeta"},
		{Name: "pinhidden", Handler: PinProbeHandler{}}, // headless: no page to open
	} {
		if _, already := SpecFor(s.Name); already {
			continue
		}
		if err := Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
}

// The order is the reader's. Sorting it would be the old alphabetical sidebar
// wearing a new name.
func TestPinnedKeepsTheOrderItWasGiven(t *testing.T) {
	registerPinProbes(t)

	got := Pinned([]string{"pinbeta", "pinalpha"})
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}
	if got[0].Name != "pinbeta" || got[1].Name != "pinalpha" {
		t.Errorf("order was rearranged: got %s, %s", got[0].Name, got[1].Name)
	}
}

// A name that no longer resolves is dropped, not rendered. Removing a service
// from an instance should quietly leave everyone's sidebar shorter rather than
// leaving a link that 404s.
func TestPinnedDropsWhatCannotBeOpened(t *testing.T) {
	registerPinProbes(t)

	got := Pinned([]string{"pinalpha", "nosuchservice", "pinhidden", "pinalpha"})
	if len(got) != 1 {
		var names []string
		for _, s := range got {
			names = append(names, s.Name)
		}
		t.Fatalf("got %v, want only pinalpha — a missing service, a headless one and a repeat should all be dropped", names)
	}
	if got[0].Name != "pinalpha" {
		t.Errorf("kept %s", got[0].Name)
	}
}

// Nothing pinned is the default and must cost nothing.
func TestPinnedIsEmptyByDefault(t *testing.T) {
	if got := Pinned(nil); len(got) != 0 {
		t.Errorf("an empty selection produced %d services", len(got))
	}
}
