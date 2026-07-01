package blog

import (
	"strings"
	"testing"
)

func TestLinkifyProtectsCurrencyDollarsFromMathRendering(t *testing.T) {
	got := Linkify("Daily Digest: AI startup raised $1 billion while BTC traded at $94,000.")
	for _, bad := range []string{"$1", "$94"} {
		if strings.Contains(got, bad) {
			t.Fatalf("Linkify left currency sequence %q exposed to math rendering: %q", bad, got)
		}
	}
	for _, want := range []string{"$\u20601 billion", "$\u206094,000"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Linkify() missing protected currency %q in %q", want, got)
		}
	}
}
