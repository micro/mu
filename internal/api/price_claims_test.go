package api

import (
	"regexp"
	"strings"
	"testing"
)

// No tool states its own price in prose.
//
// stream_post's description said "Costs 1 credit" while its operation was
// priced at 0, so /tools rendered "Included" next to a sentence claiming
// otherwise. A price written into a description is a copy that nothing updates
// when the real number moves — and the real number is an env var an operator
// can change per instance. Every surface renders the price from the operation;
// the description says what the tool does.
func TestNoToolDescriptionStatesAPrice(t *testing.T) {
	// "credits (what calls are charged in)" and similar are fine — the ban is
	// on a specific number, not on the word.
	claim := regexp.MustCompile(`(?i)\b\d+\s+credits?\b|\bcosts?\s+\d|\bfree\s+to\s+call\b|[$£]\d`)

	for _, tool := range sortedTools() {
		if m := claim.FindString(tool.Description); m != "" {
			t.Errorf("%s states a price in its description (%q); prices are rendered from the wallet operation", tool.Name, strings.TrimSpace(m))
		}
		for _, p := range tool.Params {
			if m := claim.FindString(p.Description); m != "" {
				t.Errorf("%s param %q states a price (%q)", tool.Name, p.Name, strings.TrimSpace(m))
			}
		}
	}
}
