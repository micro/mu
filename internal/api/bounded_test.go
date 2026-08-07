package api

// One tool call must not be able to cost more than the conversation it was
// meant to help. About twenty list tools grow with use and have no limit
// parameter; rather than adding twenty parameters — twenty things to document,
// to get right, and to forget on the twenty-first tool — the bound sits at the
// one place every result passes through.

import (
	"strings"
	"testing"
)

// The common case is untouched. A bound that trimmed ordinary answers would be
// a bug, not a safeguard.
func TestAnOrdinaryResultIsReturnedWhole(t *testing.T) {
	for _, in := range []string{"", "ok", strings.Repeat("a line\n", 100)} {
		if got := bounded(in); got != in {
			t.Errorf("a %d-byte result was altered", len(in))
		}
	}
}

// A silent truncation is read as the whole answer, which is the one outcome
// worse than a long one.
func TestATruncatedResultSaysSo(t *testing.T) {
	got := bounded(strings.Repeat("x", maxResultBytes*2))

	if len(got) > maxResultBytes+300 {
		t.Errorf("the bound did not hold: %d bytes", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Error("the result was cut without saying so")
	}
	if !strings.Contains(got, "limit") {
		t.Error("the notice does not say how to ask for less")
	}
}

// Cutting mid-record turns the last entry of a list into something that looks
// like data and is not. Whole lines, or nothing.
func TestTruncationFallsOnALineBoundary(t *testing.T) {
	line := strings.Repeat("y", 80) + "\n"
	got := bounded(strings.Repeat(line, (maxResultBytes/len(line))+50))

	body := got[:strings.Index(got, "[truncated")]
	for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		if l == "" {
			continue
		}
		if len(l) != 80 {
			t.Fatalf("a line was cut in half: %d chars", len(l))
		}
	}
}

// A single enormous line has no boundary to fall on, and must still be bounded
// rather than returned whole.
func TestASingleLongLineIsStillBounded(t *testing.T) {
	got := bounded(strings.Repeat("z", maxResultBytes*3))
	if len(got) > maxResultBytes+300 {
		t.Errorf("one long line escaped the bound: %d bytes", len(got))
	}
}
