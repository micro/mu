// Package transit is public transport: which stops are near you, what is coming
// next at one, and which lines are in trouble.
//
// Named for the domain, not the provider — the same reason web is not "brave".
// TfL is behind it today and a second city would go behind the same three
// methods rather than beside them.
//
// Distinct from routes and places, which are the other two travel-shaped
// services and answer different questions. routes says how to get there and how
// long it will take by car; places says what is around you; this says when the
// next bus actually turns up. Different questions, different providers, and
// keeping them apart means none of them needs another's key.
//
// It needs no key at all, which is the point. Every endpoint used here answers
// an unauthenticated request, so this works on a fresh `mu --serve` on somebody's
// laptop the moment they clone it — unlike the tools behind Atlas or Twilio,
// which make a self-hosted instance a little more hollow every time one is
// added. A capability that needs no account is worth more to this project than
// an equally useful one that does. TFL_APP_KEY raises the rate limit for an
// operator who wants it, and is free to register.
package transit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/settings"
)

// tflBase is the TfL Unified API.
const tflBase = "https://api.tfl.gov.uk"

var client = &http.Client{Timeout: 20 * time.Second}

// userAgent identifies this instance to TfL. See the note in get().
const userAgent = "mu/1.0 (+https://github.com/micro/mu)"

// get fetches a TfL path and decodes it.
//
// The app key is appended when an operator has set one. Without it TfL still
// answers, just with a lower rate limit — so an instance nobody configured
// works, and one that gets busy can be given a free key.
func get(path string, query url.Values) ([]byte, error) {
	if query == nil {
		query = url.Values{}
	}
	if k := strings.TrimSpace(settings.Get("TFL_APP_KEY")); k != "" {
		query.Set("app_key", k)
	}
	u := tflBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// TfL refuses Go's default User-Agent with a 403 — not a rate limit, not a
	// missing key, just the string "Go-http-client/1.1". The same request with
	// any other agent, or none at all, answers 200. Worth knowing because the
	// failure looks exactly like being blocked for something you did.
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transport data is unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("transport data is rate limited right now; an operator can set TFL_APP_KEY to raise it")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transport data returned %d", resp.StatusCode)
	}
	return body, nil
}

// stop is a place you can wait at.
type stop struct {
	ID       string   `json:"id"`
	Name     string   `json:"commonName"`
	Modes    []string `json:"modes"`
	Distance float64  `json:"distance"`
	Lines    []struct {
		Name string `json:"name"`
	} `json:"lines"`
}

// arrival is one service on its way.
type arrival struct {
	Line        string `json:"lineName"`
	Destination string `json:"destinationName"`
	Towards     string `json:"towards"`
	Platform    string `json:"platformName"`
	Seconds     int    `json:"timeToStation"`
	Mode        string `json:"modeName"`
}

// nearbyStops returns stops within radius metres of a point, nearest first.
func nearbyStops(lat, lon float64, radius int) ([]stop, error) {
	if radius <= 0 {
		radius = 400
	}
	if radius > 2000 {
		radius = 2000
	}
	body, err := get("/StopPoint", url.Values{
		"lat":       {fmt.Sprintf("%f", lat)},
		"lon":       {fmt.Sprintf("%f", lon)},
		"radius":    {fmt.Sprintf("%d", radius)},
		"stopTypes": {"NaptanMetroStation,NaptanRailStation,NaptanPublicBusCoachTram"},
	})
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		StopPoints []stop `json:"stopPoints"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("could not read the stop list: %w", err)
	}
	return wrapped.StopPoints, nil
}

// findStop resolves a stop name to its id.
//
// So a caller can say "Oxford Circus" rather than "940GZZLUOXC". An agent that
// must know a NaPTAN code before it can ask when the bus is has not been given
// a usable tool.
func findStop(name string) (stop, error) {
	body, err := get("/StopPoint/Search/"+url.PathEscape(strings.TrimSpace(name)), nil)
	if err != nil {
		return stop{}, err
	}
	var res struct {
		Matches []struct {
			ID    string   `json:"id"`
			Name  string   `json:"name"`
			Modes []string `json:"modes"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return stop{}, fmt.Errorf("could not read the search results: %w", err)
	}
	if len(res.Matches) == 0 {
		return stop{}, fmt.Errorf("no stop found called %q — this covers London only", name)
	}
	m := res.Matches[0]
	return stop{ID: m.ID, Name: m.Name, Modes: m.Modes}, nil
}

// arrivalsAt returns what is coming, soonest first.
func arrivalsAt(id string) ([]arrival, error) {
	body, err := get("/StopPoint/"+url.PathEscape(id)+"/Arrivals", nil)
	if err != nil {
		return nil, err
	}
	var out []arrival
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("could not read the arrivals: %w", err)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Seconds < out[i].Seconds {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// lineStatus is how a line is doing.
type lineStatus struct {
	Name      string
	Status    string
	Reason    string
	Disrupted bool
}

// statuses returns the current state of every line on the given modes.
func statuses(modes string) ([]lineStatus, error) {
	if strings.TrimSpace(modes) == "" {
		modes = "tube,dlr,overground,elizabeth-line,tram"
	}
	body, err := get("/Line/Mode/"+url.PathEscape(modes)+"/Status", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name         string `json:"name"`
		LineStatuses []struct {
			Description string `json:"statusSeverityDescription"`
			Reason      string `json:"reason"`
			Severity    int    `json:"statusSeverity"`
		} `json:"lineStatuses"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("could not read the line statuses: %w", err)
	}

	out := make([]lineStatus, 0, len(raw))
	for _, l := range raw {
		if len(l.LineStatuses) == 0 {
			continue
		}
		s := l.LineStatuses[0]
		// TfL grades severity 0–20, where 10 is "Good Service" and anything
		// below it is worse. Reporting the number would make a caller learn
		// TfL's scale; reporting the flag says the thing they wanted to know.
		out = append(out, lineStatus{
			Name:      l.Name,
			Status:    s.Description,
			Reason:    strings.TrimSpace(s.Reason),
			Disrupted: s.Severity < 10,
		})
	}
	return out, nil
}

// mins renders seconds-to-arrival the way a departure board does.
func mins(seconds int) string {
	m := seconds / 60
	switch {
	case m <= 0:
		return "due"
	case m == 1:
		return "1 min"
	}
	return fmt.Sprintf("%d mins", m)
}
