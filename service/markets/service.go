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
	Category string `json:"category" description:"crypto, stocks, futures, commodities or currencies (default crypto)"`
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
	Endpoints: map[string]service.Endpoint{
		"List": {Doc: "Get live prices for cryptocurrencies, stocks, futures, commodities and currencies"},
	},
}
