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
	"net/url"
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

	b.WriteString(`<div class="page-stack"><section class="page-section"><h3>Nutrition and ingredients</h3><p>Find a packaged food to see its nutritional information, ingredients and recorded allergens.</p><form class="search-bar" method="get" action="/food">`)
	fmt.Fprintf(&b, `<input type="search" name="q" value="%s" placeholder="Search food or brand" aria-label="Food or brand"><button type="submit">Search</button></form>`, html.EscapeString(find))
	b.WriteString(`<details class="disclosure"><summary>Look up a barcode</summary><form class="search-bar" method="get" action="/food">`)
	fmt.Fprintf(&b, `<input type="text" inputmode="numeric" name="barcode" value="%s" placeholder="Barcode on the packet" aria-label="Barcode"><button type="submit">Look up</button></form></details></section>`, html.EscapeString(barcode))

	switch {
	case barcode != "":
		var rsp ProductResponse
		if err := (Server{}).Product(r.Context(), &ProductRequest{Barcode: barcode}, &rsp); err != nil {
			b.WriteString(errorBlock(err))
		} else {
			b.WriteString(textBlock(rsp.Text))
		}
	case find != "":
		hits, err := search(find, 12)
		if err != nil {
			b.WriteString(errorBlock(err))
		} else {
			b.WriteString(`<div class="compact-list">`)
			for _, h := range hits {
				fmt.Fprintf(&b, `<a class="list-link compact-list-item" href="/food?barcode=%s"><strong>%s</strong><span class="text-muted">%s</span><span class="text-sm">View nutritional information</span></a>`, url.QueryEscape(h.Code), html.EscapeString(h.Name), html.EscapeString(strings.TrimSpace(h.Brands+" "+h.Quantity)))
			}
			if len(hits) == 0 {
				b.WriteString(`<p>No matching foods found.</p>`)
			}
			b.WriteString(`</div>`)
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

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{
		Title:       "Food",
		Description: "Nutrition, ingredients and allergens",
		HTML:        b.String(),
	})
}

// textBlock renders a service answer, which is plain text with meaningful
// line breaks.
func textBlock(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return `<div class="pre-wrap">` + html.EscapeString(s) + `</div>`
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
	return `<p class="card-desc">Find nutritional information, ingredients and allergens for packaged foods.</p><p><a href="/food">Find food</a></p>`
}
