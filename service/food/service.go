// Package food is what you are about to eat.
//
// Two questions, from two public databases, neither of which wants a key. What
// is in this packet — the barcode, the ingredients, the allergens — from Open
// Food Facts. And whether the kitchen that cooked it is clean, from the Food
// Standards Agency's published inspections.
//
// Both are the same kind of thing as hazards and transit: authoritative rather
// than plausible. A model asked whether a biscuit contains nuts, or whether the
// cafe on the corner passed its inspection, will produce a confident paragraph
// from training data. These produce the label and the inspector's score.
//
// Where the data is silent this is silent too. An absence of allergen
// information is not an absence of allergens, and a business with no rating has
// not failed one — both are facts about the database, and reporting them as
// facts about the food would be the one way this could actually hurt somebody.
package food

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the service handler. Its methods are exposed as RPC endpoints and,
// through the agent and gateways, as AI tools.
type Server struct{}

// ── Product ─────────────────────────────────────────────────────────────────

// ProductRequest is the barcode on the packet.
type ProductRequest struct {
	Barcode string `json:"barcode" required:"true" description:"The barcode, 6 to 14 digits, e.g. 5000168034928"`
}

// ProductResponse is what is in it.
type ProductResponse struct {
	Text string `json:"text" description:"Name, brand, ingredients, allergens and nutrition per 100g"`
}

// Product looks up a packaged food by its barcode.
// @example {"barcode": "5000168034928"}
func (Server) Product(_ context.Context, req *ProductRequest, rsp *ProductResponse) error {
	p, err := byBarcode(req.Barcode)
	if err != nil {
		return err
	}

	var b strings.Builder
	name := p.Name
	if name == "" {
		name = "Unnamed product"
	}
	b.WriteString(name)
	if p.Brands != "" {
		b.WriteString(" — " + p.Brands)
	}
	if p.Quantity != "" {
		b.WriteString(", " + p.Quantity)
	}
	b.WriteString("\n")

	// Allergens first. It is the only line here somebody might be unable to
	// eat, so it is never below the calorie count.
	if len(p.Allergens) > 0 {
		fmt.Fprintf(&b, "Contains: %s\n", strings.Join(p.Allergens, ", "))
	}
	if len(p.Traces) > 0 {
		fmt.Fprintf(&b, "May contain: %s\n", strings.Join(p.Traces, ", "))
	}
	if len(p.Allergens) == 0 && len(p.Traces) == 0 {
		// Said explicitly, because a missing line reads as "no allergens" and
		// this database is contributed to by the public.
		b.WriteString("Allergens: not recorded — check the packet\n")
	}

	if p.Ingredients != "" {
		fmt.Fprintf(&b, "Ingredients: %s\n", p.Ingredients)
	}
	if p.Vegan != "" || p.Vegetarian != "" {
		var tags []string
		if p.Vegan != "" {
			tags = append(tags, "vegan: "+p.Vegan)
		}
		if p.Vegetarian != "" {
			tags = append(tags, "vegetarian: "+p.Vegetarian)
		}
		fmt.Fprintf(&b, "%s\n", strings.Join(tags, ", "))
	}

	if n := p.Per100g; len(n) > 0 {
		b.WriteString("Per 100g:")
		for _, k := range []struct{ key, label, unit string }{
			{"energy-kcal", "energy", "kcal"},
			{"fat", "fat", "g"},
			{"saturated-fat", "saturates", "g"},
			{"carbohydrates", "carbs", "g"},
			{"sugars", "sugars", "g"},
			{"fiber", "fibre", "g"},
			{"proteins", "protein", "g"},
			{"salt", "salt", "g"},
		} {
			if v, ok := n[k.key]; ok {
				fmt.Fprintf(&b, " %s %g%s,", k.label, round1(v), k.unit)
			}
		}
		b.WriteString("\n")
	}

	var grades []string
	if p.Nutriscore != "" && p.Nutriscore != "NOT-APPLICABLE" && p.Nutriscore != "UNKNOWN" {
		grades = append(grades, "Nutri-Score "+p.Nutriscore)
	}
	if w := novaWord(p.Nova); w != "" {
		grades = append(grades, w)
	}
	if len(grades) > 0 {
		fmt.Fprintf(&b, "%s\n", strings.Join(grades, ", "))
	}

	rsp.Text = strings.TrimRight(strings.ReplaceAll(b.String(), ",\n", "\n"), "\n")
	return nil
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

// ── Search ──────────────────────────────────────────────────────────────────

// SearchRequest names what to look for.
type SearchRequest struct {
	Query string `json:"query" required:"true" description:"Product or brand name, e.g. 'hobnobs' or 'oat milk'"`
	Limit int    `json:"limit" description:"How many to return, default 10, max 25"`
}

// SearchResponse is what was found.
type SearchResponse struct {
	Text string `json:"text" description:"Matching products with their barcodes, for looking one up in full"`
}

// Search finds packaged foods by name.
// @example {"query": "oat milk"}
func (Server) Search(_ context.Context, req *SearchRequest, rsp *SearchResponse) error {
	hits, err := search(req.Query, req.Limit)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		rsp.Text = fmt.Sprintf("Nothing found for %q.", req.Query)
		return nil
	}

	var b strings.Builder
	for _, h := range hits {
		b.WriteString(h.Name)
		if h.Brands != "" {
			b.WriteString(" — " + h.Brands)
		}
		if h.Quantity != "" {
			b.WriteString(", " + h.Quantity)
		}
		if h.Nutriscore != "" && len(h.Nutriscore) == 1 {
			b.WriteString(" (Nutri-Score " + h.Nutriscore + ")")
		}
		// The barcode is printed because it is what food_product takes.
		fmt.Fprintf(&b, "\n  barcode %s\n", h.Code)
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
}

// ── Hygiene ─────────────────────────────────────────────────────────────────

// HygieneRequest names a business or a place to look around.
type HygieneRequest struct {
	Name  string  `json:"name" description:"Business name, e.g. 'Nandos'"`
	Where string  `json:"where" description:"Town, street or postcode to narrow it to"`
	Lat   float64 `json:"lat" description:"Optional: look around this point instead"`
	Lon   float64 `json:"lon" description:"Optional: look around this point instead"`
	Limit int     `json:"limit" description:"How many to return, default 10, max 30"`
}

// HygieneResponse is what the inspectors found.
type HygieneResponse struct {
	Text string `json:"text" description:"Businesses with their hygiene rating and when it was given"`
}

// Hygiene reports food hygiene ratings for UK businesses.
// @example {"name": "Nandos", "where": "Lincoln"}
func (Server) Hygiene(_ context.Context, req *HygieneRequest, rsp *HygieneResponse) error {
	var places []place
	var err error
	if req.Lat != 0 || req.Lon != 0 {
		places, err = ratedNear(req.Lat, req.Lon, req.Name, req.Limit)
	} else {
		places, err = ratedByName(req.Name, req.Where, req.Limit)
	}
	if err != nil {
		return err
	}
	if len(places) == 0 {
		rsp.Text = "Nothing found. This covers the United Kingdom, where every food " +
			"business is inspected and the ratings are published."
		return nil
	}

	var b strings.Builder
	for _, p := range places {
		b.WriteString(p.Name)
		fmt.Fprintf(&b, " — %s", p.ratingWord())
		if !p.RatedOn.IsZero() {
			fmt.Fprintf(&b, ", inspected %s", p.RatedOn.Format("Jan 2006"))
		}
		b.WriteString("\n")
		var where []string
		if p.Address != "" {
			where = append(where, p.Address)
		}
		if p.Postcode != "" {
			where = append(where, p.Postcode)
		}
		if len(where) > 0 {
			fmt.Fprintf(&b, "  %s", strings.Join(where, ", "))
			if p.HaveAway {
				fmt.Fprintf(&b, " (%.1fkm)", p.AwayKm)
			}
			b.WriteString("\n")
		}
		if s := p.scoreWords(); s != "" {
			fmt.Fprintf(&b, "  %s\n", s)
		}
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("food", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "food",
	Handler:     new(Server),
	Description: "What is in it and whether the kitchen is clean",
	Page:        "/food",
	Icon:        "food.svg",
	Card:        service.Glance(Card),
	Endpoints: map[string]service.Endpoint{
		"Product": {
			Doc: "Look up a packaged food by its barcode — name, brand, ingredients, " +
				"allergens, nutrition per 100g, Nutri-Score and how processed it is",
		},
		"Search": {
			Doc: "Find packaged foods by name or brand, with the barcode of each so one " +
				"can then be looked up in full",
		},
		"Hygiene": {
			Doc: "Food hygiene ratings for UK businesses, from the Food Standards Agency " +
				"inspections. Takes a business name, a town or postcode, or a lat/lon to " +
				"look around",
		},
	},
}
