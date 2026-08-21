package markets

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The From box offers what the tab is about, and only what converts.
//
// It was a text field defaulting to GBP, so the crypto tab — ten coins listed
// directly above it — offered to convert pounds. A dropdown built blindly from
// the tab would be worse on the other side: only fiat and crypto have a unit
// price in USD, so the stocks tab would list eleven tickers that each answer
// "we do not price that against anything".
func TestTheConverterOffersWhatTheTabIsAbout(t *testing.T) {
	for _, tc := range []struct {
		category string
		wants    []string
		wantsNot []string
		firstIs  string
	}{
		{category: "crypto", wants: []string{">BTC<", ">ETH<", ">USD<"}, firstIs: "BTC"},
		{category: "", wants: []string{">BTC<", ">USD<"}, firstIs: "BTC"},
		{category: "currencies", wants: []string{">GBP<", ">EUR<"}, wantsNot: []string{">BTC<"}, firstIs: "EUR"},
		{category: "stocks", wants: []string{">GBP<"}, wantsNot: []string{">AAPL<", ">BTC<"}, firstIs: "EUR"},
		{category: "commodities", wantsNot: []string{">OIL<", ">GOLD<"}, firstIs: "EUR"},
	} {
		got := converterHTML(httptest.NewRequest("GET", "/markets?category="+tc.category, nil))

		if !strings.Contains(got, `<select class="fx-code" name="from"`) {
			t.Errorf("%s: the From box is not a dropdown", tc.category)
		}
		for _, w := range tc.wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s: the converter does not offer %s", tc.category, w)
			}
		}
		for _, w := range tc.wantsNot {
			if strings.Contains(got, w) {
				t.Errorf("%s: the converter offers %s, which it cannot convert", tc.category, w)
			}
		}
		if codes := convertibleFor(tc.category); codes[0] != tc.firstIs {
			t.Errorf("%s: defaults to %s, want %s", tc.category, codes[0], tc.firstIs)
		}
	}
}

// What was chosen survives, even when it is not on the list.
//
// A code can arrive in the URL — a bookmark, a link in a message — and a form
// that silently swaps it for the first option answers a different question from
// the one asked.
func TestTheConverterKeepsACodeItDoesNotList(t *testing.T) {
	got := converterHTML(httptest.NewRequest("GET", "/markets?category=crypto&from=XMR&to=USD&amount=1", nil))
	if !strings.Contains(got, `<option value="XMR" selected>XMR</option>`) {
		t.Errorf("a code that is not on the list was dropped from the form:\n%s", got)
	}
}
