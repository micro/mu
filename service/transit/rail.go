package transit

// Trains, from National Rail's Live Departure Boards.
//
// The other half of "what is due". TfL answers for London and the published
// timetable answers everywhere, and between them there was no live answer for a
// train outside the M25 — which is most trains.
//
// # Why this and not the Darwin feed
//
// Darwin's real-time feed is the same data upstream and is a Kafka consumer
// group: a firehose of every movement on the network, with offsets to manage
// and a connection to hold open, which you then have to store and index before
// it can answer a question. That is a different kind of program from this one.
// Everything in this repository is request in, response out — see "What a
// service is" in CLAUDE.md — and a service that only works while a background
// consumer has been running is one that answers differently depending on how
// long the process has been up.
//
// LDBWS is the same information shaped as a question: which trains are leaving
// this station, now. It is SOAP, which is unfashionable and completely fine —
// encoding/xml reads it, there is nothing to keep running between calls, and a
// restart loses nothing.
//
// The trade is real and worth writing down: this cannot answer "where is
// 1A23 right now" across the whole network, because nothing here is watching the
// whole network. It answers about a station. That is what a departure board is.

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/internal/settings"
)

// ldbEndpoint is the Live Departure Boards staff-free service.
const ldbEndpoint = "https://lite.realtime.nationalrail.co.uk/OpenLDBWS/ldb11.asmx"

// RailConfigured reports whether this instance can ask about trains.
func RailConfigured() bool { return settings.Get("LDBWS_TOKEN") != "" }

// train is one service on a board.
type train struct {
	When      string // scheduled time, "14:32"
	Expected  string // "On time", "14:38", "Cancelled"
	Platform  string
	Operator  string
	Where     string // destination for a departure, origin for an arrival
	Late      bool
	Cancelled bool
}

// ldbEnvelope is the SOAP response, read down to the fields a board shows.
//
// Named by the XML rather than by the wire namespaces, because encoding/xml
// matches on local names here and the namespace has been revised four times
// without the fields changing.
type ldbEnvelope struct {
	Body struct {
		Departures struct {
			Result stationBoard `xml:"GetStationBoardResult"`
		} `xml:"GetDepartureBoardResponse"`
		Arrivals struct {
			Result stationBoard `xml:"GetStationBoardResult"`
		} `xml:"GetArrivalBoardResponse"`
		Fault struct {
			Reason string `xml:"faultstring"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// stationBoard is one board, and it is one type on purpose: departures and
// arrivals come back in the same shape and were briefly written out twice,
// which is how the two copies had already drifted by a field before either was
// ever read.
type stationBoard struct {
	LocationName string   `xml:"locationName"`
	CRS          string   `xml:"crs"`
	NRCCMessages []string `xml:"nrccMessages>message"`
	Services     []struct {
		STD      string `xml:"std"`
		ETD      string `xml:"etd"`
		STA      string `xml:"sta"`
		ETA      string `xml:"eta"`
		Platform string `xml:"platform"`
		Operator string `xml:"operator"`
		Origin   []struct {
			Name string `xml:"locationName"`
		} `xml:"origin>location"`
		Destination []struct {
			Name string `xml:"locationName"`
		} `xml:"destination>location"`
	} `xml:"trainServices>service"`
}

// railBoard asks for departures or arrivals at a station.
//
// One function for both because the request differs by a word and the response
// differs by which time field is filled in — two nearly identical functions
// would drift, and the drift would be in the error handling nobody reads.
func railBoard(crs string, rows int, arrivals bool) (string, []train, []string, error) {
	token := strings.TrimSpace(settings.Get("LDBWS_TOKEN"))
	if token == "" {
		return "", nil, nil, fmt.Errorf("this instance has no National Rail token, so it " +
			"cannot ask about trains — an admin can set LDBWS_TOKEN under Transit in Settings")
	}
	crs = strings.ToUpper(strings.TrimSpace(crs))
	if len(crs) != 3 {
		return "", nil, nil, fmt.Errorf("%q is not a station code — they are three letters, "+
			"like KGX for King's Cross or MAN for Manchester Piccadilly", crs)
	}
	if rows <= 0 || rows > 20 {
		rows = 10
	}

	op := "GetDepartureBoard"
	if arrivals {
		op = "GetArrivalBoard"
	}
	body := fmt.Sprintf(soapRequest, token, op, rows, crs, op)

	req, err := http.NewRequest(http.MethodPost, ldbEndpoint, strings.NewReader(body))
	if err != nil {
		return "", nil, nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://thalesgroup.com/RTTI/2012-01-13/ldb/"+op)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, nil, fmt.Errorf("could not reach National Rail: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var env ldbEnvelope
	if err := xml.NewDecoder(resp.Body).Decode(&env); err != nil {
		return "", nil, nil, fmt.Errorf("could not read the board: %w", err)
	}
	if f := strings.TrimSpace(env.Body.Fault.Reason); f != "" {
		// The one worth translating: an expired or wrong token comes back as a
		// SOAP fault about validation, which tells an operator nothing.
		if strings.Contains(strings.ToLower(f), "token") {
			return "", nil, nil, fmt.Errorf("National Rail rejected the token — check " +
				"LDBWS_TOKEN under Transit in Settings")
		}
		return "", nil, nil, fmt.Errorf("National Rail said: %s", f)
	}

	result := env.Body.Departures.Result
	if arrivals {
		result = env.Body.Arrivals.Result
	}
	if result.LocationName == "" {
		return "", nil, nil, fmt.Errorf("no station with the code %s", crs)
	}

	out := make([]train, 0, len(result.Services))
	for _, s := range result.Services {
		t := train{Platform: s.Platform, Operator: s.Operator}
		if arrivals {
			t.When, t.Expected = s.STA, s.ETA
			if len(s.Origin) > 0 {
				t.Where = s.Origin[0].Name
			}
		} else {
			t.When, t.Expected = s.STD, s.ETD
			if len(s.Destination) > 0 {
				t.Where = s.Destination[0].Name
			}
		}
		// "On time" and a time are different states and read differently; a
		// board that says "14:38" where the timetable says 14:32 is the one
		// somebody needs to notice.
		e := strings.ToLower(strings.TrimSpace(t.Expected))
		t.Cancelled = e == "cancelled"
		t.Late = e != "on time" && e != "" && !t.Cancelled
		out = append(out, t)
	}
	return result.LocationName, out, result.NRCCMessages, nil
}

// soapRequest is the envelope. The namespaces are the current ones and are not
// worth abstracting: when National Rail revises them it is a one-line change
// here and a compile-time nothing anywhere else.
const soapRequest = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
 xmlns:typ="http://thalesgroup.com/RTTI/2013-11-28/Token/types"
 xmlns:ldb="http://thalesgroup.com/RTTI/2021-11-01/ldb/">
 <soap:Header><typ:AccessToken><typ:TokenValue>%s</typ:TokenValue></typ:AccessToken></soap:Header>
 <soap:Body><ldb:%sRequest><ldb:numRows>%d</ldb:numRows><ldb:crs>%s</ldb:crs></ldb:%sRequest></soap:Body>
</soap:Envelope>`

// renderBoard is a departure board as text.
func renderBoard(station string, trains []train, notices []string, arrivals bool) string {
	word := "Departures from "
	if arrivals {
		word = "Arrivals at "
	}
	if len(trains) == 0 {
		return "No trains " + strings.ToLower(strings.TrimSpace(word)) + " " + station + " right now."
	}

	var b strings.Builder
	b.WriteString(word + station + ":\n")
	for _, t := range trains {
		line := "- " + t.When + " " + t.Where
		switch {
		case t.Cancelled:
			line += " — CANCELLED"
		case t.Late:
			line += " — expected " + t.Expected
		default:
			line += " — on time"
		}
		if t.Platform != "" {
			line += ", platform " + t.Platform
		}
		if t.Operator != "" {
			line += " (" + t.Operator + ")"
		}
		b.WriteString(line + "\n")
	}
	// Disruption notices last, because they are prose and the board is the
	// answer. They are worth carrying: "no service between X and Y" is the
	// thing that makes every time above it wrong.
	for _, n := range notices {
		if n = strings.TrimSpace(stripTags(n)); n != "" {
			b.WriteString("\n" + n + "\n")
		}
	}
	return b.String()
}

// stripTags removes the markup National Rail puts in disruption messages.
//
// They arrive as HTML — links to the operator's site, mostly — and this answer
// goes to an agent and to a text renderer, neither of which wants an anchor
// tag. The text inside is the message.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
