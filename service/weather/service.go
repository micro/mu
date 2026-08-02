package weather

import (
	"context"

	"mu/internal/service"
	"mu/service/wallet"
)

// Server is the go-micro service handler for weather. Its methods are exposed
// as RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// ForecastRequest is the input for a forecast lookup.
type ForecastRequest struct {
	Lat float64 `json:"lat" description:"Latitude of the location"`
	Lon float64 `json:"lon" description:"Longitude of the location"`
}

// ForecastResponse is a model-ready weather summary.
type ForecastResponse struct {
	Summary string `json:"summary" description:"Current conditions plus the next few days"`
}

// Forecast returns the weather forecast for a location (current conditions
// plus the next few days).
// @example {"lat": 51.5074, "lon": -0.1278}
func (Server) Forecast(_ context.Context, req *ForecastRequest, rsp *ForecastResponse) error {
	rsp.Summary = ForecastText(req.Lat, req.Lon)
	return nil
}

var Spec = service.Spec{
	Name:        "weather",
	Handler:     new(Server),
	Description: "Forecast, conditions and pollen",
	Page:        "/weather",
	Endpoints: map[string]service.Endpoint{
		"Forecast": {Doc: "Get the weather forecast for a location — current conditions and the days ahead", Cost: wallet.OpWeatherForecast},
	},
}
