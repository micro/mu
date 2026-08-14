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

func TestWrongTypeMatchesAnyTypeComplaintAndNothingElse(t *testing.T) {
	for _, yes := range []error{
		errors.New(`bad arguments: json: cannot unmarshal number into Go struct field ReadRequest.id of type string`),
		errors.New(`json: cannot unmarshal string into Go struct field ForecastRequest.lat of type float64`),
		errors.New(`json: cannot unmarshal bool into Go struct field SearchRequest.query of type string`),
	} {
		if !wrongType(yes) {
			t.Errorf("a type complaint was not recognised: %v", yes)
		}
	}
	for _, no := range []error{
		errors.New("no post with that id"),
		errors.New("connection refused"),
		errors.New("unauthorized"),
	} {
		if wrongType(no) {
			t.Errorf("retried on an unrelated failure: %v", no)
		}
	}
}

// The conversions the schema asks for, and no others.
func TestAsTypeConvertsOnlyWhatItCan(t *testing.T) {
	for _, c := range []struct {
		in   any
		want string
		out  any
	}{
		// The case that stranded every numeric id.
		{int64(1786633600633959421), "string", "1786633600633959421"},
		{51.5, "string", "51.5"},
		{true, "string", "true"},

		// And the mirror image: a search for the word "true", which coerce
		// turned into a boolean.
		{"true", "string", "true"},
		{"51.5", "number", 51.5},
		{"10", "integer", int64(10)},
		{"true", "boolean", true},

		// A word is not a number. The server's complaint about the type is
		// more use than a silent zero.
		{"x402", "number", "x402"},
		{"x402", "boolean", "x402"},

		// A type the schema does not name is left alone.
		{"x402", "", "x402"},
		{int64(3), "object", int64(3)},
	} {
		if got := asType(c.in, c.want); got != c.out {
			t.Errorf("asType(%#v, %q) = %#v (%T), want %#v (%T)",
				c.in, c.want, got, got, c.out, c.out)
		}
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
