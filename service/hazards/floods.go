package hazards

// Flood warnings, from the Environment Agency.
//
// The one hazard here that is a forecast rather than a record. An earthquake is
// something that happened; a flood warning is somebody saying it is going to,
// which is a different kind of fact and the more useful one — you cannot act on
// a magnitude, and you can act on "this river is expected over its banks
// tonight".
//
// Free and keyless, like USGS. England only, which is a real limit and is said
// in the answer rather than hidden: Scotland is SEPA, Wales is Natural
// Resources Wales, and both publish separately. An empty answer for Cardiff
// should not read as "no flooding in Cardiff".
//
// # Severity
//
// The Environment Agency's own four levels, and they are not decoration. A
// severe flood warning means danger to life and is a different sentence from an
// alert, which means be prepared. Flattening them into "warning" would lose the
// only distinction anybody acts on differently.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// floodAPI is the real-time flood-monitoring service. No key, no account.
const floodAPI = "https://environment.data.gov.uk/flood-monitoring/id/floods"

// severity is what the Environment Agency means by 1 to 4.
//
// Written out rather than passed through, because "severityLevel: 2" is not an
// answer and the words are the whole point of the scale.
var severity = map[int]string{
	1: "Severe flood warning — danger to life",
	2: "Flood warning — flooding expected, act now",
	3: "Flood alert — flooding possible, be prepared",
	4: "No longer in force",
}

// flood is one warning.
type flood struct {
	Area     string
	Message  string
	Level    int
	Raised   time.Time
	Distance float64 // km from the point asked about, when one was given
}

// FloodsRequest narrows what is asked for.
type FloodsRequest struct {
	Lat      float64 `json:"lat" description:"Optional: only warnings near this point"`
	Lon      float64 `json:"lon" description:"Optional: only warnings near this point"`
	WithinKm float64 `json:"within_km" description:"Optional: how near, in kilometres, default 30 when a point is given"`
	Severe   bool    `json:"severe" description:"Only warnings and severe warnings — leave out the be-prepared alerts"`
}

// FloodsResponse is what is expected.
type FloodsResponse struct {
	Text string `json:"text" description:"Flood warnings in force, most severe first, with the area each covers"`
}

// Floods lists the flood warnings in force in England.
//
// @example {"lat": 51.45, "lon": -2.58, "within_km": 40}
func (Server) Floods(_ context.Context, req *FloodsRequest, rsp *FloodsResponse) error {
	found, err := floodsNow(req.Lat, req.Lon, req.WithinKm, req.Severe)
	if err != nil {
		return err
	}
	rsp.Text = renderFloods(found, req.Lat != 0 || req.Lon != 0, req.Severe)
	return nil
}

// floodsNow asks the Environment Agency what is in force.
func floodsNow(lat, lon, withinKm float64, severeOnly bool) ([]flood, error) {
	url := floodAPI
	near := lat != 0 || lon != 0
	if near {
		if withinKm <= 0 {
			withinKm = 30
		}
		// The API filters by point and radius itself, which is the difference
		// between one request and downloading every warning in England to find
		// the three near a town.
		url = fmt.Sprintf("%s?lat=%.4f&long=%.4f&dist=%.0f", floodAPI, lat, lon, withinKm)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("could not reach the Environment Agency: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the Environment Agency returned %d", resp.StatusCode)
	}

	var body struct {
		Items []struct {
			Description   string `json:"description"`
			Message       string `json:"message"`
			SeverityLevel int    `json:"severityLevel"`
			TimeRaised    string `json:"timeRaised"`
			FloodArea     struct {
				Description string  `json:"description"`
				Lat         float64 `json:"lat"`
				Long        float64 `json:"long"`
			} `json:"floodArea"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("could not read the flood warnings: %w", err)
	}

	out := make([]flood, 0, len(body.Items))
	for _, it := range body.Items {
		// Level 4 is a warning that has been stood down. It is in the feed
		// because the feed is a record of what changed, and it is not an answer
		// to "is there a flood warning" — including it would say yes when the
		// true answer is that there was one and it is over.
		if it.SeverityLevel == 4 || it.SeverityLevel == 0 {
			continue
		}
		if severeOnly && it.SeverityLevel > 2 {
			continue
		}
		area := strings.TrimSpace(it.Description)
		if area == "" {
			area = strings.TrimSpace(it.FloodArea.Description)
		}
		f := flood{
			Area:    area,
			Message: strings.Join(strings.Fields(it.Message), " "),
			Level:   it.SeverityLevel,
		}
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(it.TimeRaised)); err == nil {
			f.Raised = t
		}
		if near && (it.FloodArea.Lat != 0 || it.FloodArea.Long != 0) {
			f.Distance = haversineKm(lat, lon, it.FloodArea.Lat, it.FloodArea.Long)
		}
		out = append(out, f)
	}

	// Most severe first, then nearest. Severity leads because it is what
	// decides whether somebody does anything, and a severe warning forty miles
	// away matters more than an alert down the road.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Distance < out[j].Distance
	})
	return out, nil
}

// renderFloods is what is in force, as text.
func renderFloods(floods []flood, near, severeOnly bool) string {
	where := "England"
	if near {
		where = "the area"
	}
	if len(floods) == 0 {
		what := "flood warnings or alerts"
		if severeOnly {
			what = "flood warnings"
		}
		return fmt.Sprintf("No %s in force in %s. England only — Scotland is SEPA and Wales "+
			"is Natural Resources Wales, and neither is checked here.", what, where)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d flood %s in force, most severe first:\n", len(floods),
		plural(len(floods), "warning", "warnings"))
	for _, f := range floods {
		line := "- " + severity[f.Level] + ": " + f.Area
		if f.Distance > 0 {
			line += fmt.Sprintf(" (%.0f km away)", f.Distance)
		}
		b.WriteString(line + "\n")
		if f.Message != "" {
			b.WriteString("  " + trimTo(f.Message, 240) + "\n")
		}
	}
	b.WriteString("\nEngland only — Scotland is SEPA and Wales is Natural Resources Wales.\n")
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// trimTo shortens a message on a word.
func trimTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
