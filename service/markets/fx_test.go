package markets

// Conversion has to be right about the day, not just the number.
//
// The ECB publishes once a business day, so a rate always belongs to a date,
// and the date it belongs to is not always the date asked for. A caller
// reconciling a Sunday transaction against Friday's rate needs to be told
// which one they were given — silently answering with Friday is how a
// reconciliation goes wrong twice.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// stubFrankfurter answers rate lookups from a table of date to rate.
func stubFrankfurter(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := frankfurterURL
	frankfurterURL = srv.URL
	t.Cleanup(func() { frankfurterURL = old })

	// The currency list is fetched once per process and remembered, so a test
	// that ran first would otherwise pin every later one to its stub.
	oldCodes := fiatCodesCache
	fiatCodesCache = map[string]string{
		"GBP": "British Pound", "USD": "United States Dollar",
		"EUR": "Euro", "JPY": "Japanese Yen",
	}
	fiatCodesOnce.Do(func() {})
	t.Cleanup(func() { fiatCodesCache = oldCodes })
}

// rates answers any request with one rate and one date.
func rates(date string, rate float64, to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "currencies") {
			w.Write([]byte(`{"GBP":"British Pound","USD":"United States Dollar","EUR":"Euro","JPY":"Japanese Yen"}`))
			return
		}
		w.Write([]byte(`{"date":"` + date + `","rates":{"` + to + `":` + trimFloat(rate) + `}}`))
	}
}

// trimFloat writes a rate the way JSON would.
//
// Deliberately not money(): that trims trailing zeros for display, which turns
// a rate of 95000 into "95" and would make this helper quietly lie.
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func TestAWeekendRateSaysWhichDayItIsFrom(t *testing.T) {
	// Asked for a Sunday, the ECB answers with the preceding Friday.
	stubFrankfurter(t, rates("2026-08-07", 1.345, "USD"))

	var rsp ConvertResponse
	if err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 100, From: "GBP", To: "USD", Date: "2026-08-09"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "7 Aug 2026") {
		t.Errorf("did not say which day the rate came from:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "nothing was published on 9 Aug 2026") {
		t.Errorf("did not say the asked-for day had no rate:\n%s", rsp.Text)
	}
}

func TestAnOrdinaryRateDoesNotApologise(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))

	var rsp ConvertResponse
	if err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 100, From: "GBP", To: "USD", Date: "2026-08-13"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rsp.Text, "nothing was published") {
		t.Errorf("claimed a fallback that did not happen:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "134.92 USD") {
		t.Errorf("wrong conversion:\n%s", rsp.Text)
	}
}

func TestCodesAreCaseInsensitive(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))

	var rsp ConvertResponse
	if err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 1, From: "gbp", To: " usd "}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "1 GBP = 1.3492 USD") {
		t.Errorf("did not normalise the codes:\n%s", rsp.Text)
	}
}

func TestConvertingSomethingToItselfIsNotALookup(t *testing.T) {
	// No stub: reaching the network here would itself be the bug.
	old := frankfurterURL
	frankfurterURL = "http://127.0.0.1:1"
	t.Cleanup(func() { frankfurterURL = old })

	var rsp ConvertResponse
	if err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 5, From: "GBP", To: "GBP"}, &rsp); err != nil {
		t.Fatalf("asked upstream what a pound is worth in pounds: %v", err)
	}
	if !strings.Contains(rsp.Text, "5 GBP = 5 GBP") {
		t.Errorf("got %q", rsp.Text)
	}
	if strings.Contains(rsp.Text, "Central Bank") {
		t.Errorf("cited a source for an identity:\n%s", rsp.Text)
	}
}

func TestCryptoConvertsThroughTheDollar(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))
	marketsMutex.Lock()
	old := cachedPrices
	cachedPrices = map[string]float64{"BTC": 95000}
	marketsMutex.Unlock()
	t.Cleanup(func() {
		marketsMutex.Lock()
		cachedPrices = old
		marketsMutex.Unlock()
	})

	var rsp ConvertResponse
	if err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 0.5, From: "BTC", To: "USD"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "47500.00 USD") {
		t.Errorf("wrong crypto conversion:\n%s", rsp.Text)
	}
}

func TestSmallAmountsSurviveRounding(t *testing.T) {
	marketsMutex.Lock()
	old := cachedPrices
	cachedPrices = map[string]float64{"BTC": 95000}
	marketsMutex.Unlock()
	t.Cleanup(func() {
		marketsMutex.Lock()
		cachedPrices = old
		marketsMutex.Unlock()
	})
	stubFrankfurter(t, rates("2026-08-13", 1, "USD"))

	var rsp ConvertResponse
	if err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 0.0004, From: "BTC", To: "USD"}, &rsp); err != nil {
		t.Fatal(err)
	}
	// Two decimal places would render the amount as 0.00 and read as nothing.
	if strings.Contains(rsp.Text, "0.00 BTC") {
		t.Errorf("rounded a real amount away to nothing:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "0.0004 BTC") {
		t.Errorf("lost the amount:\n%s", rsp.Text)
	}
}

func TestCryptoHasNoHistoryAndSaysSo(t *testing.T) {
	stubFrankfurter(t, rates("2020-01-03", 1.3096, "USD"))
	marketsMutex.Lock()
	old := cachedPrices
	cachedPrices = map[string]float64{"BTC": 95000}
	marketsMutex.Unlock()
	t.Cleanup(func() {
		marketsMutex.Lock()
		cachedPrices = old
		marketsMutex.Unlock()
	})

	var rsp ConvertResponse
	err := (Server{}).Convert(context.Background(),
		&ConvertRequest{From: "BTC", To: "USD", Date: "2020-01-03"}, &rsp)
	if err == nil {
		t.Fatalf("answered a 2020 question with today's price: %q", rsp.Text)
	}
	if !strings.Contains(err.Error(), "BTC") {
		t.Errorf("did not name the side without history: %v", err)
	}
}

func TestRefusesDatesItCannotHave(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))

	for _, c := range []struct{ name, date, want string }{
		{"future", "2099-01-01", "in the future"},
		{"unparseable", "January 2020", "2020-01-03"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var rsp ConvertResponse
			err := (Server{}).Convert(context.Background(),
				&ConvertRequest{From: "GBP", To: "USD", Date: c.date}, &rsp)
			if err == nil {
				t.Fatalf("accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want something about %q", err, c.want)
			}
		})
	}
}

func TestAColdPriceTableBlamesItselfNotTheSymbol(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))
	marketsMutex.Lock()
	old := cachedPrices
	cachedPrices = map[string]float64{}
	marketsMutex.Unlock()
	t.Cleanup(func() {
		marketsMutex.Lock()
		cachedPrices = old
		marketsMutex.Unlock()
	})

	var rsp ConvertResponse
	err := (Server{}).Convert(context.Background(),
		&ConvertRequest{Amount: 1, From: "BTC", To: "USD"}, &rsp)
	if err == nil {
		t.Fatal("converted from an empty price table")
	}
	if strings.Contains(err.Error(), "not an asset we price") {
		t.Errorf("told the caller bitcoin is not priced, on a minute-old instance: %v", err)
	}
	if !strings.Contains(err.Error(), "no live prices loaded yet") {
		t.Errorf("did not explain the cold start: %v", err)
	}
}

func TestTheConverterDoesNotFireOnArrival(t *testing.T) {
	// Landing on /markets must not spend a request at the ECB on the reader's
	// behalf — the unreachable URL is the assertion.
	old := frankfurterURL
	frankfurterURL = "http://127.0.0.1:1"
	t.Cleanup(func() { frankfurterURL = old })

	got := converterHTML(httptest.NewRequest("GET", "/markets?category=crypto", nil))
	if strings.Contains(got, "fx-result") {
		t.Errorf("converted without being asked:\n%s", got)
	}
	if !strings.Contains(got, `name="from"`) {
		t.Errorf("did not render the form:\n%s", got)
	}
}

func TestTheConverterAnswersInThePage(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 214.96, "JPY"))

	got := converterHTML(httptest.NewRequest("GET", "/markets?amount=250&from=gbp&to=jpy", nil))
	if !strings.Contains(got, "53740.00 JPY") {
		t.Errorf("wrong or missing answer:\n%s", got)
	}
}

func TestTheConverterRefusesRubbishAmounts(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))

	got := converterHTML(httptest.NewRequest("GET", "/markets?amount=lots&from=GBP&to=USD", nil))
	if !strings.Contains(got, "not a number") {
		t.Errorf("accepted a non-numeric amount:\n%s", got)
	}
}

func TestTheConverterEscapesWhatItEchoes(t *testing.T) {
	stubFrankfurter(t, rates("2026-08-13", 1.3492, "USD"))

	got := converterHTML(httptest.NewRequest("GET", "/markets?amount=1&from=%22%3E%3Cscript%3E&to=USD", nil))
	if strings.Contains(got, "<script>") {
		t.Errorf("echoed a script tag into the page:\n%s", got)
	}
}
