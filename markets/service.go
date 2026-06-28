package markets

import "context"

// Server is the go-micro service handler for markets. Its methods are exposed
// as RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// PricesRequest selects a market category.
type PricesRequest struct {
	Category string `json:"category" description:"crypto, futures, commodities or currencies (default crypto)"`
}

// PricesResponse is a model-ready price summary.
type PricesResponse struct {
	Text string `json:"text" description:"Live prices for the requested category"`
}

// Prices returns live market prices for cryptocurrencies, futures, commodities
// and currencies.
// @example {"category": "crypto"}
func (Server) Prices(_ context.Context, req *PricesRequest, rsp *PricesResponse) error {
	rsp.Text = MarketsText(req.Category)
	return nil
}
