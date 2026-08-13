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

// converterHTML renders the converter, and its answer when one was asked for.
func converterHTML(r *http.Request) string {
	q := r.URL.Query()
	from := strings.ToUpper(strings.TrimSpace(q.Get("from")))
	to := strings.ToUpper(strings.TrimSpace(q.Get("to")))
	date := strings.TrimSpace(q.Get("on"))
	amountStr := strings.TrimSpace(q.Get("amount"))

	// Defaults that make the empty form look like an example rather than a
	// puzzle: somebody who presses Convert without typing gets a real answer.
	if from == "" {
		from = "GBP"
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
		html.EscapeString(q.Get("category")))
	fmt.Fprintf(&sb, `<input class="fx-amount" type="text" name="amount" value="%s" aria-label="Amount">`,
		html.EscapeString(amountStr))
	fmt.Fprintf(&sb, `<input class="fx-code" type="text" name="from" value="%s" aria-label="From currency" maxlength="5">`,
		html.EscapeString(from))
	sb.WriteString(`<span class="fx-in">to</span>`)
	fmt.Fprintf(&sb, `<input class="fx-code" type="text" name="to" value="%s" aria-label="To currency" maxlength="5">`,
		html.EscapeString(to))
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
