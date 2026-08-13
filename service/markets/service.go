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
	// Enumerated, because a list of category names tells a caller nothing about
	// what is in them — which is how an agent asked for the oil price chose
	// "commodities", got coffee and wheat, and reported oil unavailable while
	// it sat in another category.
	Category string `json:"category" description:"crypto (BTC, ETH, SOL…), stocks, commodities (OIL, GOLD, SILVER, COPPER, COFFEE, WHEAT, CORN, SOYBEANS, OATS), futures (the metals and oil alone) or currencies (EUR, GBP, JPY…). Default crypto"`
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
		"List": {Aliases: []string{"markets"}, Doc: "Get live prices for cryptocurrencies, stocks, commodities (oil, gold, silver, copper and crops), futures and currencies"},
	},
}
