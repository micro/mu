package places

// Is it open now.
//
// The question a person standing on a street actually asks. Google resolves it
// and parseGooglePlaces was throwing the answer away — regularOpeningHours has
// been in the field mask since that file was written, so this was a fetched
// fact dropped on the floor rather than a capability to buy.

import (
	"strings"
	"testing"
)

func TestOpenNowSurvivesTheParse(t *testing.T) {
	open := true
	shut := false

	results := []googlePlaceResult{
		{
			ID: "a", DisplayName: &struct {
				Text string `json:"text"`
			}{Text: "Open Cafe"},
			Location: struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}{51.5, -0.1},
			RegularOpeningHours: &struct {
				OpenNow bool `json:"openNow"`
			}{OpenNow: open},
		},
		{
			ID: "b", DisplayName: &struct {
				Text string `json:"text"`
			}{Text: "Shut Cafe"},
			Location: struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}{51.5, -0.1},
			RegularOpeningHours: &struct {
				OpenNow bool `json:"openNow"`
			}{OpenNow: shut},
		},
		{
			ID: "c", DisplayName: &struct {
				Text string `json:"text"`
			}{Text: "Unknown Cafe"},
			Location: struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}{51.5, -0.1},
		},
	}

	got := parseGooglePlaces(results)
	if len(got) != 3 {
		t.Fatalf("%d places came back", len(got))
	}
	if got[0].Open != "open" {
		t.Errorf("an open place is %q", got[0].Open)
	}
	if got[1].Open != "closed" {
		t.Errorf("a closed place is %q", got[1].Open)
	}
	// Three states, not two. A place with no hours known must not be reported
	// shut, which is what a bare false would have said.
	if got[2].Open != "" {
		t.Errorf("a place with no hours known is %q, want nothing", got[2].Open)
	}
}

// And the answer reaches the caller, before the reference material. Somebody
// deciding whether to walk somewhere reads this and stops.
func TestTheAnswerIsInWhatTheCallerReads(t *testing.T) {
	text := renderPlaces("coffee", []*Place{
		{Name: "Open Cafe", Address: "1 High St", Open: "open", OpeningHours: "Mo-Fr 07:00-19:00"},
		{Name: "Shut Cafe", Address: "2 High St", Open: "closed"},
		{Name: "Quiet Cafe", Address: "3 High St", OpeningHours: "Mo-Su 08:00-18:00"},
	}, false)

	if !strings.Contains(text, "Open Cafe — 1 High St · open now · Mo-Fr 07:00-19:00") {
		t.Errorf("the open place does not read right:\n%s", text)
	}
	if !strings.Contains(text, "Shut Cafe — 2 High St · closed now") {
		t.Errorf("the closed place does not read right:\n%s", text)
	}
	// Unknown says nothing about being open, and still gives the raw hours it
	// does have. Nothing tries to derive an answer from an OSM tag: that syntax
	// has holidays, seasons and sunset-relative times in it, and being told a
	// place is open when it is shut is the failure that matters.
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "Quiet Cafe") {
			line = l
		}
	}
	if strings.Contains(line, "now") {
		t.Errorf("a place with no resolved hours claimed to know: %q", line)
	}
	if !strings.Contains(line, "Mo-Su 08:00-18:00") {
		t.Errorf("the raw hours were dropped: %q", line)
	}
}
