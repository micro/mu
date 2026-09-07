package flights

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/settings"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const statusCost = quota.OpFlightStatus

var statusClient = &http.Client{Timeout: 12 * time.Second}
var statusURL = "https://api.aviationstack.com/v1/flights"
var flightCode = regexp.MustCompile(`^[A-Z0-9]{2}[0-9]{1,4}[A-Z]?$`)
var airportCode = regexp.MustCompile(`^[A-Z]{3}$`)

type StatusRequest struct {
	Flight    string `json:"flight" description:"IATA flight number, e.g. BA117"`
	Airport   string `json:"airport" description:"IATA airport code, e.g. LHR"`
	Direction string `json:"direction" description:"arrivals or departures (default arrivals)"`
}
type FlightTime struct {
	Airport   string `json:"airport"`
	IATA      string `json:"iata"`
	Scheduled string `json:"scheduled"`
	Estimated string `json:"estimated"`
	Actual    string `json:"actual"`
	Terminal  string `json:"terminal"`
	Gate      string `json:"gate"`
}
type FlightStatus struct {
	Date      string     `json:"flight_date"`
	Status    string     `json:"flight_status"`
	Departure FlightTime `json:"departure"`
	Arrival   FlightTime `json:"arrival"`
	Flight    struct {
		IATA string `json:"iata"`
	} `json:"flight"`
}
type StatusResponse struct {
	Flights   []FlightStatus `json:"flights"`
	FetchedAt string         `json:"fetched_at"`
}

func (Server) Status(ctx context.Context, req *StatusRequest, rsp *StatusResponse) error {
	var err error
	rsp.Flights, err = flightStatus(req)
	if err == nil {
		rsp.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return err
}
func flightStatus(in *StatusRequest) ([]FlightStatus, error) {
	key := settings.Get("AVIATIONSTACK_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("arrival and departure times are not configured on this instance")
	}
	q := url.Values{"access_key": {key}, "limit": {"20"}}
	flight := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in.Flight), " ", ""))
	airport := strings.ToUpper(strings.TrimSpace(in.Airport))
	if flight != "" {
		if !flightCode.MatchString(flight) {
			return nil, fmt.Errorf("use an IATA flight number such as BA117")
		}
		q.Set("flight_iata", flight)
	} else {
		if !airportCode.MatchString(airport) {
			return nil, fmt.Errorf("use a three-letter airport code such as LHR")
		}
		switch in.Direction {
		case "", "arrivals":
			q.Set("arr_iata", airport)
		case "departures":
			q.Set("dep_iata", airport)
		default:
			return nil, fmt.Errorf("choose arrivals or departures")
		}
	}
	req, _ := http.NewRequest(http.MethodGet, statusURL+"?"+q.Encode(), nil)
	resp, err := statusClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach flight status")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("flight status is unavailable (%d)", resp.StatusCode)
	}
	var data struct {
		Data  []FlightStatus  `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("could not read flight status")
	}
	if len(data.Error) > 0 && string(data.Error) != "null" {
		return nil, fmt.Errorf("flight status provider rejected the lookup; check the configured plan")
	}
	if len(data.Data) > 20 {
		data.Data = data.Data[:20]
	}
	return data.Data, nil
}
func statusForm(r *http.Request) string {
	b := `<section class="card"><h3>Arrivals and departures</h3>`
	if settings.Get("AVIATIONSTACK_API_KEY") == "" {
		return b + `<p class="text-sm text-muted">Timetable and estimated times are unavailable on this instance. Live aircraft positions remain available below.</p></section>`
	}
	b += `<form method="POST" action="/flights" class="fl-form">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="status_lookup" value="1"><div class="fl-row"><input name="flight" placeholder="Flight number, e.g. BA117" aria-label="Flight number"><input name="airport" placeholder="Airport, e.g. LHR" aria-label="Airport"><select name="direction" aria-label="Flight direction"><option value="arrivals">Arrivals</option><option value="departures">Departures</option></select><button>Check times</button></div><div class="d-flex gap-2 mt-3">`
	for _, code := range []string{"LHR", "LGW", "MAN", "JFK", "CDG", "DXB"} {
		b += `<button name="preset" value="` + code + `">` + code + `</button>`
	}
	return b + `</div><p class="text-sm text-muted">Up to 20 provider results per lookup. Times include the airport's UTC offset; estimates may change.</p></form></section>`
}
func statusPage(w http.ResponseWriter, r *http.Request) {
	owner, ok := app.BillableCaller(w, r, statusCost)
	if !ok {
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	in := StatusRequest{Flight: r.PostFormValue("flight"), Airport: r.PostFormValue("airport"), Direction: r.PostFormValue("direction")}
	if code := r.PostFormValue("preset"); code != "" {
		in.Flight = ""
		in.Airport = code
	}
	if owner != "" {
		if err := auth.CheckPostRate(owner); err != nil {
			app.RespondError(w, 429, "Please wait before another lookup.")
			return
		}
	}

	flights, err := flightStatus(&in)
	if err == nil && owner != "" {
		err = quota.Charge(owner, statusCost, nil)
	}
	b := statusForm(r)
	if err != nil {
		b += `<p role="alert">` + html.EscapeString(err.Error()) + `</p>`
	} else if len(flights) == 0 {
		b += `<p>No matching flight records. This does not mean a flight is cancelled.</p>`
	} else {
		for _, f := range flights {
			b += `<section class="card"><h3>` + html.EscapeString(f.Flight.IATA+" · "+f.Date+" · "+f.Status) + `</h3>`
			for _, part := range []struct {
				name string
				v    FlightTime
			}{{"Departure", f.Departure}, {"Arrival", f.Arrival}} {
				b += `<p><strong>` + part.name + ` · ` + html.EscapeString(part.v.Airport+" ("+part.v.IATA+")") + `</strong></p><ul>`
				for _, t := range []struct{ label, value string }{{"Scheduled", part.v.Scheduled}, {"Estimated", part.v.Estimated}, {"Actual", part.v.Actual}, {"Terminal", part.v.Terminal}, {"Gate", part.v.Gate}} {
					value := t.value
					if value == "" {
						value = "Not available"
					}
					b += `<li>` + t.label + `: ` + html.EscapeString(value) + `</li>`
				}
				b += `</ul>`
			}
			b += `</section>`
		}
		b += `<p class="text-sm text-muted">Source: Aviationstack · fetched ` + time.Now().UTC().Format("15:04 UTC") + `</p>`
	}
	b += `<p><a href="/flights">← Flights</a></p>`
	app.Respond(w, r, app.Response{Title: "Flights", HTML: b + pageCSS})
}
