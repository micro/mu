package cli

// A number typed on a command line is a number.
//
// Everything off argv is a string and the tools are not, so
// `lat=51.5` arrived as "51.5" and the server refused it with "cannot
// unmarshal string into Go struct field ForecastRequest.lat".
//
// That is not merely annoying on this path. The payment settles at the door,
// before the tool runs and therefore before anything reads the arguments — so
// a mistyped number was a charge with no answer. Verified with real USDC: the
// call errored and the balance still went down.

import "testing"

func TestTypedArgReadsWhatItLooksLike(t *testing.T) {
	for _, c := range []struct {
		in   string
		want any
	}{
		{"51.5", 51.5},
		{"-0.12", -0.12},
		{"10", int64(10)},
		{"-3", int64(-3)},
		{"0", int64(0)},
		{"true", true},
		{"false", false},

		// Words stay words. A query of "true" or "42" is ordinary, and
		// silently retyping it would be worse than the bug being fixed.
		{"x402", "x402"},
		{"", ""},
		{"51.5kg", "51.5kg"},

		// Not every string ParseFloat accepts is a number somebody typed.
		{"1e5", "1e5"},
		{"Inf", "Inf"},
		{"NaN", "NaN"},
		{"0x10", "0x10"},

		// An address must never become a float or an int.
		{"0x4160a86303eeba12fc0a3ffb8480a9d2d1eab7a1", "0x4160a86303eeba12fc0a3ffb8480a9d2d1eab7a1"},
	} {
		if got := typedArg(c.in); got != c.want {
			t.Errorf("typedArg(%q) = %#v (%T), want %#v (%T)", c.in, got, got, c.want, c.want)
		}
	}
}
