package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"mu/internal/mesh"
	"mu/internal/settings"
)

// WxProbe stands in for the weather service for this test.
type WxProbe struct{}

type WxReq struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}
type WxRsp struct {
	Summary string `json:"summary"`
}

// Forecast returns the weather for a location.
// @example {"lat": 51.5074, "lon": -0.1278}
func (WxProbe) Forecast(_ context.Context, req *WxReq, rsp *WxRsp) error {
	rsp.Summary = "Weather for London. Now: 14C, light rain, wind 20 km/h."
	return nil
}

// TestQueryNativeLive verifies the native go-micro agent path answers using the
// registered service tools. Gated on ATLAS_API_KEY.
func TestQueryNativeLive(t *testing.T) {
	key := os.Getenv("ATLAS_API_KEY")
	if key == "" {
		t.Skip("set ATLAS_API_KEY to run")
	}
	settings.Set("ATLAS_API_KEY", key)
	if err := mesh.Register("weather", WxProbe{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	answer, handled, err := queryNative("acct-1", "What's the weather in London right now?", QueryOpts{})
	if !handled {
		t.Fatal("native path did not handle the query")
	}
	if err != nil {
		t.Fatalf("queryNative: %v", err)
	}
	t.Logf("answer: %s", answer)
	if !strings.Contains(answer, "14") {
		t.Fatalf("native agent did not use the weather tool result: %q", answer)
	}
}
