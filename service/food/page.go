package food

// The page at /food, and the card.
//
// A plain form that submits to itself, like the currency converter: one
// question, one answer, and a result that has a URL so it can be sent to
// somebody. There is nothing here worth a fetch and a spinner.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
)

// Handler serves /food.
func Handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	barcode := strings.TrimSpace(q.Get("barcode"))
	find := strings.TrimSpace(q.Get("q"))
	rating := strings.TrimSpace(q.Get("rating"))

	var b strings.Builder
	b.WriteString(`<p class="card-desc">What is in a packet, and whether the kitchen is clean. ` +
		`Ingredients and allergens from Open Food Facts; hygiene ratings from the Food ` +
		`Standards Agency. Both public, neither needs a key.</p>`)

	b.WriteString(`<form class="food-form" method="get" action="/food">`)
	fmt.Fprintf(&b, `<input class="food-input" type="text" name="q" value="%s" placeholder="Find a product — oat milk" aria-label="Product name">`,
		html.EscapeString(find))
	fmt.Fprintf(&b, `<input class="food-input food-code" type="text" name="barcode" value="%s" placeholder="or a barcode" aria-label="Barcode">`,
		html.EscapeString(barcode))
	b.WriteString(`<button class="btn" type="submit">Look up</button>`)
	b.WriteString(`</form>`)

	b.WriteString(`<form class="food-form" method="get" action="/food">`)
	fmt.Fprintf(&b, `<input class="food-input" type="text" name="rating" value="%s" placeholder="Hygiene rating — a business name" aria-label="Business name">`,
		html.EscapeString(rating))
	fmt.Fprintf(&b, `<input class="food-input" type="text" name="where" value="%s" placeholder="town or postcode" aria-label="Where">`,
		html.EscapeString(q.Get("where")))
	b.WriteString(`<button class="btn" type="submit">Check</button>`)
	b.WriteString(`</form>`)

	switch {
	case barcode != "":
		var rsp ProductResponse
		if err := (Server{}).Product(r.Context(), &ProductRequest{Barcode: barcode}, &rsp); err != nil {
			b.WriteString(errorBlock(err))
		} else {
			b.WriteString(textBlock(rsp.Text))
		}
	case find != "":
		var rsp SearchResponse
		if err := (Server{}).Search(r.Context(), &SearchRequest{Query: find, Limit: 12}, &rsp); err != nil {
			b.WriteString(errorBlock(err))
		} else {
			b.WriteString(textBlock(rsp.Text))
		}
	case rating != "" || strings.TrimSpace(q.Get("where")) != "":
		var rsp HygieneResponse
		err := (Server{}).Hygiene(r.Context(), &HygieneRequest{
			Name: rating, Where: q.Get("where"), Limit: 12}, &rsp)
		if err != nil {
			b.WriteString(errorBlock(err))
		} else {
			b.WriteString(textBlock(rsp.Text))
		}
	}

	app.Respond(w, r, app.Response{
		Title:       "Food",
		Description: "Ingredients, allergens and nutrition by barcode, and UK food hygiene ratings",
		HTML:        b.String(),
	})
}

// textBlock renders a service answer, which is plain text with meaningful
// line breaks.
func textBlock(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return `<pre class="food-result">` + html.EscapeString(s) + `</pre>`
}

func errorBlock(err error) string {
	return `<p class="food-result text-error">` + html.EscapeString(err.Error()) + `</p>`
}

// Card is the summary shown on the home screen.
//
// Deliberately not a live lookup: there is no "current food", and firing a
// request at a free database on every page render to show a fixed sentence
// would be rude.
func Card() string {
	return `<p class="card-desc">Scan a barcode for ingredients and allergens, ` +
		`or check a restaurant's hygiene rating.</p>` +
		`<p><a href="/food">Look something up</a></p>`
}
