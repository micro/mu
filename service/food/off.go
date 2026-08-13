package food

// What is actually in it, from Open Food Facts.
//
// A cooperative database of about three million packaged products, built the
// way OpenStreetMap was and published under the same kind of licence. Keyless,
// which is the reason it is here: a barcode is the most ordinary question
// somebody standing in a shop has, and it should not need an account.
//
// The point is allergens. A model asked whether a biscuit contains nuts will
// produce a confident paragraph; this produces the label. Where the label is
// silent, so is this — an absence of allergen data is not an absence of
// allergens, and that distinction is the whole reason to prefer the database
// over the paragraph.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	offProductURL = "https://world.openfoodfacts.org/api/v2/product/"
	// Search goes to the project's dedicated search service rather than to the
	// old cgi endpoint on the main site. The old one is rate limited hard
	// enough to be useless: measured at two requests before it started
	// answering 503, where this one took five in a row without complaint.
	offSearchURL = "https://search.openfoodfacts.org/search"
)

// userAgent identifies this instance. Open Food Facts asks callers to name
// themselves, and it is good manners to anybody running a free database.
const userAgent = "mu/1.0 (+https://github.com/micro/mu)"

var client = &http.Client{Timeout: 25 * time.Second}

// product is one item as the database has it.
type product struct {
	Code        string
	Name        string
	Brands      string
	Quantity    string
	Ingredients string
	Allergens   []string
	Traces      []string
	// Nutriscore is A to E, and NOVA is 1 to 4 — how processed it is.
	Nutriscore string
	Nova       int
	Per100g    map[string]float64
	Vegan      string
	Vegetarian string
}

func get(u string, into any) error {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("the food database is unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf("the food database is busy — try again in a moment")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the food database returned %d", resp.StatusCode)
	}
	return json.Unmarshal(body, into)
}

// barcodeOK rejects anything that is not a barcode before spending a request.
func barcodeOK(code string) bool {
	if len(code) < 6 || len(code) > 14 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// byBarcode looks a product up by the number on the packet.
func byBarcode(code string) (*product, error) {
	code = strings.TrimSpace(code)
	if !barcodeOK(code) {
		return nil, fmt.Errorf("that is not a barcode — it should be 6 to 14 digits, like 5000168034928")
	}

	var raw struct {
		Status  int `json:"status"`
		Product struct {
			Name        string         `json:"product_name"`
			Brands      string         `json:"brands"`
			Quantity    string         `json:"quantity"`
			Ingredients string         `json:"ingredients_text"`
			Allergens   []string       `json:"allergens_tags"`
			Traces      []string       `json:"traces_tags"`
			Nutriscore  string         `json:"nutriscore_grade"`
			Nova        int            `json:"nova_group"`
			Nutriments  map[string]any `json:"nutriments"`
			Analysis    []string       `json:"ingredients_analysis_tags"`
		} `json:"product"`
	}
	if err := get(offProductURL+url.PathEscape(code)+".json", &raw); err != nil {
		return nil, err
	}
	if raw.Status != 1 {
		return nil, fmt.Errorf("no product with barcode %s — the database is contributed to by "+
			"the public, so a product can be genuinely absent", code)
	}

	p := &product{
		Code: code, Name: raw.Product.Name, Brands: raw.Product.Brands,
		Quantity: raw.Product.Quantity, Ingredients: raw.Product.Ingredients,
		Allergens:  cleanTags(raw.Product.Allergens),
		Traces:     cleanTags(raw.Product.Traces),
		Nutriscore: strings.ToUpper(strings.TrimSpace(raw.Product.Nutriscore)),
		Nova:       raw.Product.Nova,
		Per100g:    map[string]float64{},
	}
	for _, key := range []string{"energy-kcal_100g", "fat_100g", "saturated-fat_100g",
		"carbohydrates_100g", "sugars_100g", "fiber_100g", "proteins_100g", "salt_100g"} {
		if v, ok := numeric(raw.Product.Nutriments[key]); ok {
			p.Per100g[strings.TrimSuffix(key, "_100g")] = v
		}
	}
	for _, tag := range raw.Product.Analysis {
		switch tag {
		case "en:vegan":
			p.Vegan = "yes"
		case "en:non-vegan":
			p.Vegan = "no"
		case "en:vegetarian":
			p.Vegetarian = "yes"
		case "en:non-vegetarian":
			p.Vegetarian = "no"
		}
	}
	return p, nil
}

// numeric reads a nutriment, which arrives as a number or a string depending on
// who contributed it.
func numeric(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%g", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// cleanTags turns "en:peanuts" into "peanuts".
func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if i := strings.Index(t, ":"); i >= 0 {
			t = t[i+1:]
		}
		t = strings.ReplaceAll(t, "-", " ")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// hit is one product from a search.
type hit struct {
	Code       string
	Name       string
	Brands     string
	Quantity   string
	Nutriscore string
}

// search finds products by name.
func search(q string, limit int) ([]hit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("say what to look for")
	}
	if limit <= 0 || limit > 25 {
		limit = 10
	}

	v := url.Values{}
	v.Set("q", q)
	v.Set("page_size", fmt.Sprint(limit))
	v.Set("fields", "code,product_name,brands,quantity,nutriscore_grade")

	var raw struct {
		Hits []struct {
			Code     string   `json:"code"`
			Name     string   `json:"product_name"`
			Brands   []string `json:"brands"`
			Quantity string   `json:"quantity"`
			// Absent products come back as the string "unknown" rather than
			// as no grade at all, which would otherwise print as a rating.
			Nutriscore string `json:"nutriscore_grade"`
		} `json:"hits"`
	}
	if err := get(offSearchURL+"?"+v.Encode(), &raw); err != nil {
		return nil, err
	}

	var out []hit
	for _, p := range raw.Hits {
		if strings.TrimSpace(p.Name) == "" {
			continue // a contributed row with no name is not a search result
		}
		grade := strings.ToUpper(strings.TrimSpace(p.Nutriscore))
		if grade == "UNKNOWN" || grade == "NOT-APPLICABLE" {
			grade = ""
		}
		out = append(out, hit{
			Code: p.Code, Name: p.Name, Brands: strings.Join(p.Brands, ", "),
			Quantity: p.Quantity, Nutriscore: grade,
		})
	}
	return out, nil
}

// novaWord says what a processing group means, because the number does not.
func novaWord(n int) string {
	switch n {
	case 1:
		return "unprocessed"
	case 2:
		return "culinary ingredient"
	case 3:
		return "processed"
	case 4:
		return "ultra-processed"
	}
	return ""
}
