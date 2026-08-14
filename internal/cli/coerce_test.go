package cli

// A value off a command line has no type, and the CLI has to guess one.
//
// It guesses from the characters, because nothing here knows the tool's schema.
// That is fine until an identifier is all digits: `mu blog read --id
// 1786633600633959421` sent a JSON number, the server wanted a string, and
// there was no way to read a post by its id at all. The old advice — quote it —
// was never true, because the shell removes the quotes before we see anything.

import (
	"errors"
	"testing"
)

func TestWantedAStringOnlyMatchesThatMistake(t *testing.T) {
	yes := errors.New(`bad arguments: json: cannot unmarshal number into Go struct field ReadRequest.id of type string`)
	if !wantedAString(yes) {
		t.Error("the type mismatch that strands every numeric id was not recognised")
	}
	for _, no := range []error{
		errors.New("no post with that id"),
		errors.New(`json: cannot unmarshal string into Go struct field ForecastRequest.lat of type float64`),
		errors.New(`json: cannot unmarshal number into Go struct field X.y of type bool`),
		errors.New("connection refused"),
	} {
		if wantedAString(no) {
			t.Errorf("retried on an unrelated failure: %v", no)
		}
	}
}

func TestStringifyNumbersLeavesEverythingElseAlone(t *testing.T) {
	in := map[string]any{
		"id":     int64(1786633600633959421),
		"lat":    51.5,
		"query":  "x402",
		"public": true,
	}
	out, changed := stringifyNumbers(in)
	if !changed {
		t.Fatal("nothing was re-typed")
	}
	if out["id"] != "1786633600633959421" {
		t.Errorf(`id = %#v, want the digits as a string`, out["id"])
	}
	if out["lat"] != "51.5" {
		t.Errorf(`lat = %#v, want "51.5" — and not "51.500000"`, out["lat"])
	}
	if out["query"] != "x402" || out["public"] != true {
		t.Errorf("a string or a bool was altered: %#v", out)
	}

	// Nothing numeric, nothing to retry.
	if _, changed := stringifyNumbers(map[string]any{"query": "x402"}); changed {
		t.Error("a retry was offered for a call with no numbers in it")
	}
}

// The guess itself is unchanged, and worth pinning: the retry is a safety net,
// not a licence for coerce to get sloppier.
func TestCoerceStillReadsTheObviousTypes(t *testing.T) {
	for _, c := range []struct {
		in   string
		want any
	}{
		{"true", true},
		{"false", false},
		{"10", int64(10)},
		{"-3", int64(-3)},
		{"51.5", 51.5},
		{"x402", "x402"},
		{"", ""},
	} {
		if got := coerce(c.in); got != c.want {
			t.Errorf("coerce(%q) = %#v (%T), want %#v (%T)", c.in, got, got, c.want, c.want)
		}
	}
}
