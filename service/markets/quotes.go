package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mu/internal/service"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Price struct {
	Symbol    string    `json:"symbol"`
	Currency  string    `json:"currency"`
	Value     float64   `json:"price"`
	Change24h float64   `json:"change_24h"`
	Updated   time.Time `json:"updated"`
	Source    string    `json:"source"`
	Stale     bool      `json:"stale"`
}

func price(symbol string, p PriceData) Price {
	return Price{Symbol: symbol, Currency: "USD", Value: p.Price, Change24h: p.Change24h, Updated: p.UpdatedAt, Source: p.Source, Stale: p.UpdatedAt.IsZero() || time.Since(p.UpdatedAt) > 2*time.Hour}
}

type QuoteRequest struct {
	Symbol string `json:"symbol" required:"true" description:"One supported ticker, e.g. BTC, AAPL, GOLD or EUR; prices are in USD"`
}
type QuoteResponse struct {
	Item *Price `json:"item"`
	Text string `json:"text"`
}

// Quote returns one cached instrument with its source and freshness.
func (Server) Quote(_ context.Context, req *QuoteRequest, rsp *QuoteResponse) error {
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	p, ok := AllPriceData()[symbol]
	if !ok || p.Price == 0 {
		return fmt.Errorf("no price available for %s", symbol)
	}
	item := price(symbol, p)
	rsp.Item = &item
	rsp.Text = fmt.Sprintf("%s: %.8g USD, updated %s (%s)", symbol, p.Price, p.UpdatedAt.Format(time.RFC3339), p.Source)
	if item.Stale {
		rsp.Text += ". Cached price is stale or undated."
	}
	return nil
}

type HistoryRequest struct {
	Offset int    `json:"offset"`
	Limit  int    `json:"limit" description:"Maximum points per page, default 20, maximum 100"`
	Symbol string `json:"symbol" required:"true" description:"Supported ticker, e.g. BTC, AAPL, GOLD or EUR"`
	Start  string `json:"start" description:"First date, YYYY-MM-DD; default 30 days ago"`
	End    string `json:"end" description:"Last date inclusive, YYYY-MM-DD; default today. Maximum range 366 days"`
}
type Point struct {
	Time  time.Time `json:"time"`
	Close float64   `json:"close"`
}
type HistoryResponse struct {
	Total      int     `json:"total"`
	NextOffset *int    `json:"next_offset,omitempty"`
	Symbol     string  `json:"symbol"`
	Currency   string  `json:"currency"`
	Source     string  `json:"source"`
	Interval   string  `json:"interval"`
	Items      []Point `json:"items"`
	Text       string  `json:"text"`
}

var historyClient = &http.Client{Timeout: 15 * time.Second}
var historyURL = "https://query1.finance.yahoo.com/v8/finance/chart/"

func historySymbol(symbol string) string {
	if _, ok := cryptoGeckoIDs[symbol]; ok {
		return symbol + "-USD"
	}
	if s := futuresSymbols[symbol]; s != "" {
		return s
	}
	if s := forexSymbols[symbol]; s != "" {
		return s
	}
	for _, s := range stockSymbols {
		if s == symbol {
			return s
		}
	}
	return ""
}

// History reads bounded daily closing prices from the existing finance provider.
// Missing trading days stay missing; no prices are fabricated or interpolated.
func (Server) History(ctx context.Context, req *HistoryRequest, rsp *HistoryResponse) error {
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	provider := historySymbol(symbol)
	if provider == "" {
		return fmt.Errorf("unsupported symbol; use an instrument listed by markets_list")
	}
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -30)
	var err error
	if req.End != "" {
		end, err = time.Parse("2006-01-02", req.End)
		if err != nil {
			return fmt.Errorf("end must be YYYY-MM-DD")
		}
		start = end.AddDate(0, 0, -30)
	}
	if req.Start != "" {
		start, err = time.Parse("2006-01-02", req.Start)
		if err != nil {
			return fmt.Errorf("start must be YYYY-MM-DD")
		}
	}
	if start.After(end) || end.Sub(start) > 365*24*time.Hour || end.After(time.Now().UTC().Truncate(24*time.Hour)) {
		return fmt.Errorf("choose a past range of at most 366 days")
	}
	q := url.Values{"period1": {fmt.Sprint(start.Unix())}, "period2": {fmt.Sprint(end.AddDate(0, 0, 1).Unix())}, "interval": {"1d"}}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, historyURL+url.PathEscape(provider)+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	r.Header.Set("User-Agent", "mu/1.0")
	resp, err := historyClient.Do(r)
	if err != nil {
		return fmt.Errorf("market history unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("market history provider returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 2*1024*1024 {
		return fmt.Errorf("market history response too large")
	}
	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Currency string `json:"currency"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		} `json:"chart"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("invalid market history response")
	}
	if len(payload.Chart.Result) == 0 {
		return fmt.Errorf("no history available for %s", symbol)
	}
	result := payload.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return fmt.Errorf("no closing prices available")
	}
	rsp.Symbol, rsp.Currency, rsp.Source, rsp.Interval = symbol, result.Meta.Currency, "Yahoo Finance", "1d"
	rsp.Items = []Point{}
	closes := result.Indicators.Quote[0].Close
	for i, stamp := range result.Timestamp {
		if i >= len(closes) || closes[i] == nil {
			continue
		}
		t := time.Unix(stamp, 0).UTC()
		if t.Before(start) || !t.Before(end.AddDate(0, 0, 1)) {
			continue
		}
		if len(rsp.Items) >= 366 {
			return fmt.Errorf("provider exceeded daily history limit")
		}
		rsp.Items = append(rsp.Items, Point{t, *closes[i]})
	}
	if len(rsp.Items) == 0 {
		return fmt.Errorf("no closing prices in the requested range")
	}
	startIndex, endIndex := service.PageRange(len(rsp.Items), req.Offset, req.Limit)
	rsp.Total = len(rsp.Items)
	if endIndex < rsp.Total {
		rsp.NextOffset = &endIndex
	}
	rsp.Items = rsp.Items[startIndex:endIndex]
	rsp.Text = fmt.Sprintf("%d daily closing prices for %s in %s, from Yahoo Finance. Trading gaps are omitted; today's candle may be incomplete.", len(rsp.Items), symbol, rsp.Currency)
	return nil
}

func listPrices(req *ListRequest, rsp *ListResponse) {
	data := AllPriceData()
	items := []Price{}
	for _, symbol := range getAssetsForCategory(strings.ToLower(strings.TrimSpace(req.Category))) {
		if p, ok := data[symbol]; ok && p.Price != 0 {
			items = append(items, price(symbol, p))
		}
	}
	start, end := service.PageRange(len(items), req.Offset, req.Limit)
	rsp.Items, rsp.Total, rsp.Offset = items[start:end], len(items), start
	if end < len(items) {
		rsp.NextOffset = &end
	}
}
