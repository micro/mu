package food

// Whether the kitchen is clean, from the Food Standards Agency.
//
// Every food business in the United Kingdom is inspected and scored, and the
// scores are published for anybody to redistribute. Keyless, authoritative, and
// exactly the kind of thing a model would otherwise guess at — nobody can infer
// from training data whether the cafe on the corner passed its last inspection
// in May.
//
// Two schemes hide behind one API. England, Wales and Northern Ireland use
// FHRS, which is 0 to 5 and higher is better. Scotland uses FHIS, which has no
// number at all and says Pass or Improvement Required. Printing "rating 0" for
// a Scottish restaurant that passed would be a serious thing to get wrong, so
// the scheme is read rather than assumed.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var fsaURL = "https://api.ratings.food.gov.uk/Establishments"

// place is one inspected business.
type place struct {
	Name     string
	Kind     string
	Rating   string
	RatedOn  time.Time
	Address  string
	Postcode string
	Scheme   string
	AwayKm   float64
	HaveAway bool
	// The three component scores, where they exist.
	Hygiene, Structural, Management int
	HaveScores                      bool
}

// fsaGet queries the ratings API.
func fsaGet(v url.Values) ([]place, error) {
	req, err := http.NewRequest(http.MethodGet, fsaURL+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// The version header is not optional: without it the API answers with an
	// older shape that has none of these fields in it.
	req.Header.Set("x-api-version", "2")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the hygiene ratings are unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the hygiene ratings returned %d", resp.StatusCode)
	}

	var raw struct {
		Establishments []struct {
			Name     string   `json:"BusinessName"`
			Kind     string   `json:"BusinessType"`
			Rating   string   `json:"RatingValue"`
			RatedOn  string   `json:"RatingDate"`
			Line1    string   `json:"AddressLine1"`
			Line2    string   `json:"AddressLine2"`
			Line3    string   `json:"AddressLine3"`
			Postcode string   `json:"PostCode"`
			Scheme   string   `json:"SchemeType"`
			Distance *float64 `json:"Distance"`
			Scores   struct {
				Hygiene    *int `json:"Hygiene"`
				Structural *int `json:"Structural"`
				Management *int `json:"ConfidenceInManagement"`
			} `json:"scores"`
		} `json:"establishments"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("could not read the ratings: %w", err)
	}

	out := make([]place, 0, len(raw.Establishments))
	for _, e := range raw.Establishments {
		p := place{
			Name: strings.TrimSpace(e.Name), Kind: strings.TrimSpace(e.Kind),
			Rating: strings.TrimSpace(e.Rating), Postcode: strings.TrimSpace(e.Postcode),
			Scheme: strings.TrimSpace(e.Scheme),
		}
		p.Address = strings.Join(nonEmpty(e.Line1, e.Line2, e.Line3), ", ")
		if t, err := time.Parse("2006-01-02T15:04:05", e.RatedOn); err == nil {
			p.RatedOn = t
		}
		if e.Distance != nil {
			// The API reports miles under a name that does not say so.
			p.AwayKm, p.HaveAway = *e.Distance*1.60934, true
		}
		if s := e.Scores; s.Hygiene != nil && s.Structural != nil && s.Management != nil {
			p.Hygiene, p.Structural, p.Management = *s.Hygiene, *s.Structural, *s.Management
			p.HaveScores = true
		}
		out = append(out, p)
	}
	return out, nil
}

func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ratedNear finds inspected businesses around a point.
func ratedNear(lat, lon float64, name string, limit int) ([]place, error) {
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	v := url.Values{}
	v.Set("latitude", strconv.FormatFloat(lat, 'f', 6, 64))
	v.Set("longitude", strconv.FormatFloat(lon, 'f', 6, 64))
	v.Set("maxDistanceLimit", "2") // miles, the largest the API accepts
	v.Set("pageSize", strconv.Itoa(limit))
	v.Set("sortOptionKey", "distance")
	if n := strings.TrimSpace(name); n != "" {
		v.Set("name", n)
	}
	return fsaGet(v)
}

// ratedByName finds inspected businesses by name, optionally in a place.
func ratedByName(name, where string, limit int) ([]place, error) {
	if strings.TrimSpace(name) == "" && strings.TrimSpace(where) == "" {
		return nil, fmt.Errorf("give a business name, a place, or a point to look around")
	}
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	v := url.Values{}
	if n := strings.TrimSpace(name); n != "" {
		v.Set("name", n)
	}
	if w := strings.TrimSpace(where); w != "" {
		v.Set("address", w)
	}
	v.Set("pageSize", strconv.Itoa(limit))
	return fsaGet(v)
}

// ratingWord renders a rating in the terms its own scheme uses.
func (p place) ratingWord() string {
	r := p.Rating
	switch strings.ToLower(r) {
	case "", "awaitinginspection", "awaiting inspection":
		return "not yet inspected"
	case "exempt":
		return "exempt from rating"
	case "awaitingpublication":
		return "rated, not yet published"
	case "pass":
		return "pass"
	case "improvement required", "improvementrequired":
		return "improvement required"
	}
	// FHIS has no numbers, so anything numeric is the 0-to-5 scheme.
	if n, err := strconv.Atoi(r); err == nil {
		if n == 5 {
			return "5 out of 5 — very good"
		}
		return fmt.Sprintf("%d out of 5", n)
	}
	return r
}

// scoreWords explains the component scores, which run the other way.
//
// In FHRS a component score is how much was found wrong, so zero is the best
// possible and twenty is the worst. Printing "Hygiene: 0" without saying that
// reads as a catastrophe when it means a spotless kitchen.
func (p place) scoreWords() string {
	if !p.HaveScores {
		return ""
	}
	band := func(n int) string {
		switch {
		case n <= 0:
			return "very good"
		case n <= 5:
			return "good"
		case n <= 10:
			return "some improvement needed"
		}
		return "major improvement needed"
	}
	return fmt.Sprintf("hygiene %s, structure %s, management %s",
		band(p.Hygiene), band(p.Structural), band(p.Management))
}
