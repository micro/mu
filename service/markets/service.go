package markets

import (
	"context"

	"mu/internal/service"
)

// Server is the go-micro service handler for markets. Its methods are exposed
// as RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// ListRequest selects a market category.
type ListRequest struct {
	// The categories name themselves badly enough that a caller cannot guess
	// them, so they are enumerated. "commodities" here means the agricultural
	// ones; oil and the metals are under "futures", which is true of the
	// contracts and useless to somebody asking what oil costs — an agent asked
	// for the oil price picked commodities, got coffee and wheat, and correctly
	// reported that it could not find oil while oil was one category away.
	//
	// Listing what is in each is the cheap half of the fix. The taxonomy itself
	// is still wrong: gold is a commodity in every sense a person means it.
	Category string `json:"category" description:"crypto (BTC, ETH, SOL…), stocks, futures (OIL, GOLD, SILVER, COPPER), commodities (COFFEE, WHEAT, CORN, SOYBEANS, OATS) or currencies (EUR, GBP, JPY…). Default crypto. Oil and metals are under futures, not commodities"`
}

// ListResponse is a model-ready price summary.
type ListResponse struct {
	Text string `json:"text" description:"Live prices for the requested category"`
}

// List returns live market prices for cryptocurrencies, stocks, futures,
// commodities and currencies.
// @example {"category": "crypto"}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = MarketsText(req.Category)
	return nil
}

var Spec = service.Spec{
	Name:        "markets",
	Handler:     new(Server),
	Description: "Live crypto, stock, futures, commodity and currency prices",
	Page:        "/markets",
	Icon:        "markets.svg",
	Card:        MarketsHTML,
	Endpoints: map[string]service.Endpoint{
		"List": {Aliases: []string{"markets"}, Doc: "Get live prices for cryptocurrencies, stocks, futures (oil, gold, silver, copper), agricultural commodities and currencies"},
	},
}
