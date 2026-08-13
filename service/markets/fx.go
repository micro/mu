package markets

// Turning one currency into another.
//
// markets_list already showed eight currencies against the dollar, which is a
// dashboard rather than an answer: it tells you what a pound is worth and
// leaves you to do the arithmetic, in the wrong direction, against the wrong
// base, for a currency that might not be on the list. "What is 250 GBP in yen"
// was not answerable by this service at all.
//
// The rates come from the European Central Bank's daily reference set through
// Frankfurter — keyless, thirty-odd currencies, and published back to 1999,
// which is three things Yahoo's eight tickers are not. The dashboard keeps
// Yahoo; it wants a 24-hour change, and this wants breadth and history.
//
// Crypto is bridged through the prices this service already holds, because an
// agent with a wallet asks what its balance is worth in real money and should
// not have to make two calls and a multiplication to find out.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var frankfurterURL = "https://api.frankfurter.dev/v1"

var fxClient = &http.Client{Timeout: 20 * time.Second}

// fxRate is one conversion, and what it was based on.
type fxRate struct {
	Rate float64
	// Date the rate is actually from. The ECB does not publish at weekends or
	// on its holidays, so asking about a Sunday answers with Friday — and a
	// caller reconciling a transaction needs to know which day they got.
	Date     time.Time
	Asked    time.Time
	Fallback bool
}

// fiatRate fetches the ECB rate between two currency codes.
//
// day is the zero time for the latest published set.
func fiatRate(from, to string, day time.Time) (*fxRate, error) {
	when := "latest"
	if !day.IsZero() {
		when = day.Format("2006-01-02")
	}
	url := fmt.Sprintf("%s/%s?base=%s&symbols=%s", frankfurterURL, when, from, to)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mu/1.0 (+https://github.com/micro/mu)")

	resp, err := fxClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange rates are unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no rate for %s to %s — check the currency codes, and note that rates start in 1999", from, to)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange rates returned %d", resp.StatusCode)
	}

	var r struct {
		Date  string             `json:"date"`
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("could not read the rate: %w", err)
	}
	rate, ok := r.Rates[to]
	if !ok {
		return nil, fmt.Errorf("no rate for %s to %s", from, to)
	}

	out := &fxRate{Rate: rate, Asked: day}
	if d, err := time.Parse("2006-01-02", r.Date); err == nil {
		out.Date = d
		out.Fallback = !day.IsZero() && !d.Equal(day)
	}
	return out, nil
}

// fiatCodes is the set the ECB publishes, fetched once and remembered.
//
// Worth holding because it is the difference between "no rate for ZZZ" and
// "ZZZ is not a currency we can convert", and between a typo and a currency
// that exists but is outside the reference set.
var (
	fiatCodesOnce  sync.Once
	fiatCodesCache map[string]string
)

func fiatCodes() map[string]string {
	fiatCodesOnce.Do(func() {
		fiatCodesCache = map[string]string{}
		req, err := http.NewRequest(http.MethodGet, frankfurterURL+"/currencies", nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "mu/1.0 (+https://github.com/micro/mu)")
		resp, err := fxClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return
		}
		var m map[string]string
		if json.Unmarshal(body, &m) == nil {
			fiatCodesCache = m
		}
	})
	return fiatCodesCache
}

// isFiat reports whether a code is one the ECB set covers.
//
// Falls back to a fixed list when the currency list could not be fetched, so a
// conversion still works on an instance that started up offline.
func isFiat(code string) bool {
	if codes := fiatCodes(); len(codes) > 0 {
		_, ok := codes[code]
		return ok
	}
	switch code {
	case "EUR", "USD", "GBP", "JPY", "CHF", "AUD", "CAD", "CNY", "INR",
		"NZD", "SEK", "NOK", "DKK", "PLN", "CZK", "HUF", "SGD", "HKD",
		"KRW", "MXN", "BRL", "ZAR", "TRY", "ILS", "IDR", "MYR", "PHP",
		"THB", "RON", "ISK", "BGN":
		return true
	}
	return false
}

// cryptoUSD returns what one unit of a symbol is worth in dollars, from the
// prices this service already tracks.
func cryptoUSD(symbol string) (float64, bool) {
	if isFiat(symbol) {
		return 0, false
	}
	prices := GetAllPrices()
	p, ok := prices[symbol]
	if !ok || p <= 0 {
		return 0, false
	}
	return p, true
}

// conversion is the answer to "how much is this in that".
type conversion struct {
	Amount   float64
	From, To string
	Result   float64
	Rate     float64
	Date     time.Time
	Fallback bool
	Asked    time.Time
	ViaUSD   bool
	Source   string
}

// convert turns an amount of one currency into another.
//
// Everything routes through the dollar when either side is crypto, because
// that is the only pair both sources agree on. Fiat-to-fiat never does — the
// ECB gives a direct cross rate, and multiplying two rounded rates through a
// third currency would lose accuracy for no reason.
func convert(amount float64, from, to string, date string) (*conversion, error) {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return nil, fmt.Errorf("give a currency to convert from and one to convert to")
	}
	if amount == 0 {
		amount = 1
	}

	var day time.Time
	if d := strings.TrimSpace(date); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, fmt.Errorf("date must look like 2020-01-03")
		}
		if parsed.After(time.Now().UTC().AddDate(0, 0, 1)) {
			return nil, fmt.Errorf("that date is in the future — there is no rate for it yet")
		}
		day = parsed
	}

	c := &conversion{Amount: amount, From: from, To: to, Asked: day}

	if from == to {
		c.Rate, c.Result, c.Date, c.Source = 1, amount, time.Now().UTC(), "no conversion needed"
		return c, nil
	}

	fromFiat, toFiat := isFiat(from), isFiat(to)

	// The straightforward case, and the accurate one.
	if fromFiat && toFiat {
		r, err := fiatRate(from, to, day)
		if err != nil {
			return nil, err
		}
		c.Rate, c.Date, c.Fallback = r.Rate, r.Date, r.Fallback
		c.Result = amount * r.Rate
		c.Source = "European Central Bank reference rates"
		return c, nil
	}

	// Crypto has no history here: the price table is what things cost now, not
	// a series. Saying so beats quietly answering with today's rate for a
	// question about 2020.
	if !day.IsZero() {
		return nil, fmt.Errorf("historical rates cover currencies only — %s is priced live, with no past series here",
			nonFiatOf(from, to, fromFiat, toFiat))
	}

	fromUSD, okFrom := unitInUSD(from, fromFiat)
	if !okFrom {
		return nil, unknownCode(from)
	}
	toUSD, okTo := unitInUSD(to, toFiat)
	if !okTo {
		return nil, unknownCode(to)
	}

	c.Rate = fromUSD / toUSD
	c.Result = amount * c.Rate
	c.ViaUSD = true
	c.Date = time.Now().UTC()
	c.Source = "live prices via the dollar"
	return c, nil
}

// unitInUSD is what one of something is worth in dollars.
func unitInUSD(code string, fiat bool) (float64, bool) {
	if code == "USD" {
		return 1, true
	}
	if fiat {
		r, err := fiatRate(code, "USD", time.Time{})
		if err != nil {
			return 0, false
		}
		return r.Rate, true
	}
	return cryptoUSD(code)
}

// nonFiatOf names whichever side is not a currency, for an error message.
func nonFiatOf(from, to string, fromFiat, toFiat bool) string {
	if !fromFiat {
		return from
	}
	if !toFiat {
		return to
	}
	return from
}

// unknownCode explains what we do and do not price.
//
// It distinguishes a symbol we have never heard of from one we simply have not
// fetched yet. A minute-old instance has an empty price table, and telling
// somebody BTC is not an asset we price would be false as well as unhelpful.
func unknownCode(code string) error {
	tracked := make([]string, 0, 8)
	for symbol := range GetAllPrices() {
		if !isFiat(symbol) {
			tracked = append(tracked, symbol)
		}
	}
	if len(tracked) == 0 {
		return fmt.Errorf("no live prices loaded yet, so %s cannot be converted — "+
			"currency to currency works now, and this will once prices have been fetched", code)
	}
	sort.Strings(tracked)
	if len(tracked) > 10 {
		tracked = tracked[:10]
	}
	return fmt.Errorf("%s is not a currency we hold a rate for, and not an asset we price. "+
		"Currencies are the ECB reference set (USD, EUR, GBP, JPY and about thirty more); "+
		"priced assets include %s", code, strings.Join(tracked, ", "))
}

// money renders an amount with the precision the size of it deserves.
//
// Two decimal places is right for 250 dollars and useless for 0.0004 bitcoin,
// which would render as 0.00 and read as nothing.
func money(v float64) string {
	a := math.Abs(v)
	switch {
	case a == 0:
		return "0"
	case a < 0.0001:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.8f", v), "0"), ".")
	case a < 1:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
	case a < 1000:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", v), "0"), ".")
	}
	return fmt.Sprintf("%.2f", v)
}
