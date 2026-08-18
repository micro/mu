package markets

import (
	"context"
	"fmt"
	"strings"

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
	rsp.Text = Text(req.Category)
	return nil
}

// ConvertRequest is an amount and the two currencies it sits between.
type ConvertRequest struct {
	Amount float64 `json:"amount" description:"How much to convert. Default 1"`
	From   string  `json:"from" required:"true" description:"Currency or asset to convert from, as a code: GBP, USD, EUR, BTC…"`
	To     string  `json:"to" required:"true" description:"Currency or asset to convert to, as a code: JPY, USD, ETH…"`
	Date   string  `json:"date" description:"Optional: the rate on a past day, as 2020-01-03. Currencies only, back to 1999"`
}

// ConvertResponse is the amount in the other currency.
type ConvertResponse struct {
	Text string `json:"text" description:"The converted amount, the rate used, and the day the rate is from"`
}

// Convert turns an amount of one currency into another, today or on a past date.
// @example {"amount": 250, "from": "GBP", "to": "JPY"}
func (Server) Convert(_ context.Context, req *ConvertRequest, rsp *ConvertResponse) error {
	c, err := convert(req.Amount, req.From, req.To, req.Date)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s = %s %s", money(c.Amount), c.From, money(c.Result), c.To)
	fmt.Fprintf(&b, "\nRate: 1 %s = %s %s", c.From, money(c.Rate), c.To)

	switch {
	case c.From == c.To:
		// Nothing to date or source — saying "rate 1, from the ECB" about a
		// pound in pounds is noise dressed as rigour.
	case c.ViaUSD:
		fmt.Fprintf(&b, "\n%s, now", c.Source)
	case c.Fallback:
		// The ECB is closed at weekends and on its own holidays, so the answer
		// is Friday's. A caller reconciling a transaction has to know that.
		fmt.Fprintf(&b, "\n%s published %s — nothing was published on %s",
			c.Source, c.Date.Format("2 Jan 2006"), c.Asked.Format("2 Jan 2006"))
	default:
		fmt.Fprintf(&b, "\n%s, %s", c.Source, c.Date.Format("2 Jan 2006"))
	}
	rsp.Text = b.String()
	return nil
}

var Spec = service.Spec{
	Name:        "markets",
	Handler:     new(Server),
	Description: "Live crypto, stock, futures, commodity and currency prices, and conversion between them",
	Page:        "/markets",
	Icon:        "markets.svg",
	Card:        service.Glance(HTML),
	Endpoints: map[string]service.Endpoint{
		"List": {Aliases: []string{"markets"}, Doc: "Get live prices for cryptocurrencies, stocks, commodities (oil, gold, silver, copper and crops), futures and currencies"},
		"Convert": {Doc: "Convert an amount from one currency to another — 250 GBP in JPY. Uses European " +
			"Central Bank reference rates, and takes a past date back to 1999. Crypto converts at the " +
			"live price through the dollar"},
	},
}
