package weather

import (
	"context"
	"testing"

	"mu/internal/service"
)

// TestForecastViaMesh verifies the go-micro RPC round-trip: register the
// weather service and call it through the service registry, the same path the agent tool uses.
func TestForecastViaMesh(t *testing.T) {
	if err := service.Register("weather", new(Server)); err != nil {
		t.Fatalf("register: %v", err)
	}
	var rsp ForecastResponse
	err := service.Call(context.Background(), "weather", "Server.Forecast",
		&ForecastRequest{Lat: 51.5074, Lon: -0.1278}, &rsp)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if rsp.Summary == "" {
		t.Fatal("empty summary")
	}
	t.Logf("summary: %.60s", rsp.Summary)
}
