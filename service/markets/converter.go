package markets

// The converter on the page.
//
// A plain form that submits to itself, because the answer is one number and a
// date — there is nothing here worth a fetch, a spinner and an error path. It
// also means the result has a URL, so "250 GBP in yen" can be a bookmark or a
// link in a message, which the JavaScript version of this would not give
// anybody.

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
)

// convertibleFor is what the From box should offer on this tab.
//
// The tab's own tickers where they can be converted, and currencies otherwise.
// Only fiat and crypto have a unit price in USD — see unitInUSD — so a dropdown
// built blindly from getAssetsForCategory would fill the stocks tab with eleven
// options that all answer "we do not price AAPL against anything", which is
// worse than the text box it replaced. Currencies are always appended, because
// converting a thing into money is the question people actually have.
func convertibleFor(category string) []string {
	var out []string
	switch category {
	case CategoryCrypto, "":
		out = append(out, cryptoAssets...)
	case CategoryCurrencies:
		// Currencies alone; they are added below.
	default:
		// Stocks, futures, commodities: nothing on the tab converts.
	}
	return append(out, currencyAssets...)
}

// codeOptions renders a select, keeping whatever was chosen even when it is not
// on the list — somebody may have typed a code into the URL, and a form that
// silently changes what was asked for is worse than one showing an odd value.
func codeOptions(name, label, chosen string, codes []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<select class="fx-code" name="%s" aria-label="%s">`, name, label)
	seen := false
	for _, c := range codes {
		sel := ""
		if c == chosen {
			sel, seen = ` selected`, true
		}
		fmt.Fprintf(&sb, `<option value="%s"%s>%s</option>`, html.EscapeString(c), sel, html.EscapeString(c))
	}
	if !seen && chosen != "" {
		fmt.Fprintf(&sb, `<option value="%s" selected>%s</option>`,
			html.EscapeString(chosen), html.EscapeString(chosen))
	}
	sb.WriteString(`</select>`)
	return sb.String()
}

// converterHTML renders the converter, and its answer when one was asked for.
func converterHTML(r *http.Request) string {
	q := r.URL.Query()
	from := strings.ToUpper(strings.TrimSpace(q.Get("from")))
	to := strings.ToUpper(strings.TrimSpace(q.Get("to")))
	date := strings.TrimSpace(q.Get("on"))
	amountStr := strings.TrimSpace(q.Get("amount"))

	// Defaults that make the empty form look like an example rather than a
	// puzzle: somebody who presses Convert without typing gets a real answer.
	category := strings.TrimSpace(q.Get("category"))
	codes := convertibleFor(category)
	if from == "" {
		// The first thing on this tab, so the form is already asking the
		// question the tab is about. It was always GBP, which on the crypto tab
		// offered to convert pounds while ten coins were listed above it.
		from = codes[0]
	}
	if to == "" {
		to = "USD"
	}
	if amountStr == "" {
		amountStr = "1"
	}

	var sb strings.Builder
	sb.WriteString(`<form class="fx-form" method="get" action="/markets">`)
	fmt.Fprintf(&sb, `<input type="hidden" name="category" value="%s">`,
		html.EscapeString(category))
	fmt.Fprintf(&sb, `<input class="fx-amount" type="text" name="amount" value="%s" aria-label="Amount">`,
		html.EscapeString(amountStr))
	sb.WriteString(codeOptions("from", "From", from, codes))
	sb.WriteString(`<span class="fx-in">to</span>`)
	sb.WriteString(codeOptions("to", "To", to, currencyAssets))
	fmt.Fprintf(&sb, `<input class="fx-date" type="date" name="on" value="%s" aria-label="On this date">`,
		html.EscapeString(date))
	sb.WriteString(`<button class="btn" type="submit">Convert</button>`)
	sb.WriteString(`</form>`)

	// Only answer when somebody asked. Landing on /markets should not fire a
	// request at the ECB on the reader's behalf.
	if q.Get("from") == "" && q.Get("to") == "" && q.Get("amount") == "" {
		return sb.String()
	}

	amount, err := strconv.ParseFloat(strings.ReplaceAll(amountStr, ",", ""), 64)
	if err != nil {
		return sb.String() + fxError("That amount is not a number.")
	}

	c, err := convert(amount, from, to, date)
	if err != nil {
		return sb.String() + fxError(err.Error())
	}

	var out strings.Builder
	out.WriteString(`<div class="fx-result">`)
	fmt.Fprintf(&out, `<div class="fx-answer">%s %s</div>`,
		html.EscapeString(money(c.Result)), html.EscapeString(c.To))
	fmt.Fprintf(&out, `<div class="card-meta">1 %s = %s %s</div>`,
		html.EscapeString(c.From), html.EscapeString(money(c.Rate)), html.EscapeString(c.To))

	switch {
	case c.From == c.To:
	case c.ViaUSD:
		fmt.Fprintf(&out, `<div class="card-meta">%s, now</div>`, html.EscapeString(c.Source))
	case c.Fallback:
		fmt.Fprintf(&out, `<div class="card-meta">%s published %s — nothing was published on %s</div>`,
			html.EscapeString(c.Source), c.Date.Format("2 Jan 2006"), c.Asked.Format("2 Jan 2006"))
	default:
		fmt.Fprintf(&out, `<div class="card-meta">%s, %s</div>`,
			html.EscapeString(c.Source), c.Date.Format("2 Jan 2006"))
	}
	out.WriteString(`</div>`)

	return sb.String() + out.String()
}

func fxError(msg string) string {
	return `<div class="fx-result text-error">` + html.EscapeString(msg) + `</div>`
}
