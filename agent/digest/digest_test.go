package digest

import (
	"strings"
	"testing"
)

// The references block survived the move from packages to tools.
//
// news.Post had a Title and a URL as struct fields and a tool answers with
// text, so the links are read back out of the answer. Both shapes the tools
// here actually reply in are matched; a source that replies in neither still
// contributes its prose, which is the right way round.
func TestReferencesAreReadBackOutOfWhateverAToolAnswered(t *testing.T) {
	for _, c := range []struct {
		what      string
		text      string
		wantTitle string
		wantURL   string
	}{
		{
			"a markdown link",
			"- [AI lab releases safer assistant](https://example.com/ai): notes",
			"AI lab releases safer assistant", "https://example.com/ai",
		},
		{
			"a JSON object",
			`{"items":[{"title":"Quake off Honshu","url":"https://example.com/q","mag":6.1}]}`,
			"Quake off Honshu", "https://example.com/q",
		},
	} {
		var got []ref
		for _, re := range linkPatterns {
			for _, m := range re.FindAllStringSubmatch(c.text, -1) {
				got = append(got, ref{m[1], m[2]})
			}
		}
		if len(got) != 1 {
			t.Errorf("%s: found %d references, want 1 (%v)", c.what, len(got), got)
			continue
		}
		if got[0].title != c.wantTitle || got[0].url != c.wantURL {
			t.Errorf("%s: got %+v, want {%s %s}", c.what, got[0], c.wantTitle, c.wantURL)
		}
	}

	// And prose with no links at all yields none rather than something wrong.
	for _, re := range linkPatterns {
		if m := re.FindAllStringSubmatch("BTC 64,102 USD +1.2%\nETH 3,410 USD", -1); len(m) != 0 {
			t.Errorf("plain prose produced %d references", len(m))
		}
	}
}

func TestBuildReferences(t *testing.T) {
	got := buildReferences([]ref{
		{title: "First story", url: "https://example.com/first"},
		{title: "Second story", url: "https://example.com/second"},
	})

	wantParts := []string{
		"<details>",
		"<summary>References</summary>",
		"1. [First story](https://example.com/first)",
		"2. [Second story](https://example.com/second)",
		"</details>",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("buildReferences() missing %q in %q", part, got)
		}
	}
}

func TestBuildReferencesEmpty(t *testing.T) {
	if got := buildReferences(nil); got != "" {
		t.Fatalf("buildReferences(nil) = %q, want empty string", got)
	}
}

func TestStripPreamble(t *testing.T) {
	input := "Here is today's briefing:\n\nThe main story starts here.\nMarkets followed."
	want := "The main story starts here.\nMarkets followed."
	if got := stripPreamble(input); got != want {
		t.Fatalf("stripPreamble() = %q, want %q", got, want)
	}
}

func TestStripPreamblePreservesMarkdownHeading(t *testing.T) {
	input := "# Daily notes\nText follows."
	if got := stripPreamble(input); got != input {
		t.Fatalf("stripPreamble() = %q, want %q", got, input)
	}
}

func TestNormalizeHeadingsAddsBlankLineBeforeBody(t *testing.T) {
	input := "## Market moves\nOil rose.\n\n### Elsewhere\nGold fell."
	want := "## Market moves\n\nOil rose.\n\n### Elsewhere\n\nGold fell."
	if got := normalizeHeadings(input); got != want {
		t.Fatalf("normalizeHeadings() = %q, want %q", got, want)
	}
}

func TestCleanResponse(t *testing.T) {
	input := "Below is the digest:\n\n## Markets\nOil moved from $90 to $94."
	got := cleanResponse(input)
	if strings.Contains(strings.ToLower(got), "below is") {
		t.Fatalf("cleanResponse() kept preamble: %q", got)
	}
	if !strings.Contains(got, "## Markets\n\nOil moved") {
		t.Fatalf("cleanResponse() did not normalize heading spacing: %q", got)
	}
}
