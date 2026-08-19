package transit

// Reading a departure board, and reading a bus off a feed.
//
// Both talk to somebody else's XML, which is the one thing here that breaks
// without anybody touching it: a namespace moves, a field is renamed, and the
// decoder quietly produces zero services rather than an error. So both are
// tested against a recorded response rather than against the network.

import (
	"encoding/xml"
	"strings"
	"testing"
)

// A trimmed GetDepartureBoard response, with the shapes that matter: one on
// time, one late, one cancelled, and a disruption notice carrying markup.
const boardXML = `<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
 <soap:Body>
  <GetDepartureBoardResponse xmlns="http://thalesgroup.com/RTTI/2021-11-01/ldb/">
   <GetStationBoardResult>
    <locationName>London Kings Cross</locationName>
    <crs>KGX</crs>
    <nrccMessages><message>Services may be delayed. &lt;a href="http://x"&gt;More&lt;/a&gt;</message></nrccMessages>
    <trainServices>
     <service>
      <std>14:32</std><etd>On time</etd><platform>7</platform>
      <operator>LNER</operator>
      <destination><location><locationName>Edinburgh</locationName></location></destination>
     </service>
     <service>
      <std>14:45</std><etd>14:58</etd><platform>1</platform>
      <operator>Thameslink</operator>
      <destination><location><locationName>Cambridge</locationName></location></destination>
     </service>
     <service>
      <std>15:03</std><etd>Cancelled</etd>
      <operator>Great Northern</operator>
      <destination><location><locationName>Peterborough</locationName></location></destination>
     </service>
    </trainServices>
   </GetStationBoardResult>
  </GetDepartureBoardResponse>
 </soap:Body>
</soap:Envelope>`

func TestReadingADepartureBoard(t *testing.T) {
	var env ldbEnvelope
	if err := xml.Unmarshal([]byte(boardXML), &env); err != nil {
		t.Fatal(err)
	}
	got := env.Body.Departures.Result
	if got.LocationName != "London Kings Cross" {
		t.Errorf("station is %q", got.LocationName)
	}
	if len(got.Services) != 3 {
		t.Fatalf("%d services, want 3 — a decoder that reads none is how a moved "+
			"namespace looks", len(got.Services))
	}
	if got.Services[0].Platform != "7" || got.Services[0].Operator != "LNER" {
		t.Errorf("first service: %+v", got.Services[0])
	}
	if len(got.Services[0].Destination) == 0 ||
		got.Services[0].Destination[0].Name != "Edinburgh" {
		t.Errorf("the destination did not come through: %+v", got.Services[0].Destination)
	}
}

// On time, late and cancelled are three different things to a person standing
// on a platform, and a board that renders them alike is one nobody can read.
func TestABoardSaysWhichTrainsAreInTrouble(t *testing.T) {
	var env ldbEnvelope
	if err := xml.Unmarshal([]byte(boardXML), &env); err != nil {
		t.Fatal(err)
	}
	board := env.Body.Departures.Result
	var trains []train
	for _, s := range board.Services {
		tr := train{When: s.STD, Expected: s.ETD, Platform: s.Platform, Operator: s.Operator}
		if len(s.Destination) > 0 {
			tr.Where = s.Destination[0].Name
		}
		e := strings.ToLower(strings.TrimSpace(tr.Expected))
		tr.Cancelled = e == "cancelled"
		tr.Late = e != "on time" && e != "" && !tr.Cancelled
		trains = append(trains, tr)
	}

	out := renderBoard(board.LocationName, trains, board.NRCCMessages, false)
	for _, want := range []string{
		"14:32 Edinburgh — on time, platform 7 (LNER)",
		"14:45 Cambridge — expected 14:58",
		"15:03 Peterborough — CANCELLED",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the board is missing %q:\n%s", want, out)
		}
	}
	// The notice is carried, and the anchor tag in it is not.
	if !strings.Contains(out, "Services may be delayed") {
		t.Errorf("the disruption notice was dropped:\n%s", out)
	}
	if strings.Contains(out, "href") || strings.Contains(out, "<a") {
		t.Errorf("markup from the notice reached the answer:\n%s", out)
	}
}

// A station code is three letters. Catching it here saves a round trip and
// gives an answer somebody can act on, which "no station" does not.
func TestAStationCodeIsThreeLetters(t *testing.T) {
	t.Setenv("LDBWS_TOKEN", "test")
	for _, bad := range []string{"", "K", "KINGS CROSS", "KGXX"} {
		if _, _, _, err := railBoard(bad, 5, false); err == nil {
			t.Errorf("%q was accepted as a station code", bad)
		} else if !strings.Contains(err.Error(), "three letters") {
			t.Errorf("%q was refused without saying why: %v", bad, err)
		}
	}
}

// With no token nothing is attempted, and the error names the setting and where
// to set it — the failure an operator meets first.
func TestNoTokenSaysWhereToPutOne(t *testing.T) {
	t.Setenv("LDBWS_TOKEN", "")
	_, _, _, err := railBoard("KGX", 5, false)
	if err == nil {
		t.Fatal("a board was attempted with no token")
	}
	for _, want := range []string{"LDBWS_TOKEN", "Settings"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s: %v", want, err)
		}
	}
}
