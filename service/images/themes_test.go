package images

// The daily image rotation.

import (
	"strings"
	"testing"
)

// Every theme comes round, and none comes round twice before the rest have.
//
// The rotation strides through the list rather than stepping one at a time, so
// consecutive days are from different families. That only works while the
// stride and the length are coprime: share a factor and the rotation quietly
// collapses to a fraction of the list — with 31 themes and a stride of 7 you
// see all 31, with a stride of 7 and 28 themes you see four of them forever.
// Nothing errors, nothing logs, and somebody notices months later that it is
// always fractals.
func TestEveryThemeComesRound(t *testing.T) {
	n := len(dailyThemes)
	seen := map[string]int{}
	for day := 1; day <= n; day++ {
		seen[themeFor(day).name]++
	}
	if len(seen) != n {
		t.Errorf("%d themes in %d days, want all %d — the stride %d shares a "+
			"factor with the length", len(seen), n, n, themeStride)
	}
	for name, times := range seen {
		if times != 1 {
			t.Errorf("%s comes round %d times in one cycle", name, times)
		}
	}
}

// Consecutive days do not look alike.
//
// The point of the stride. Neighbours in the list are deliberately similar —
// dawn beside dusk, lake beside tide pools — because that is how the list is
// readable, and it is the wrong order to show them in.
func TestConsecutiveDaysAreNotNeighbours(t *testing.T) {
	at := map[string]int{}
	for i, th := range dailyThemes {
		at[th.name] = i
	}
	for day := 1; day <= len(dailyThemes); day++ {
		a, b := themeFor(day), themeFor(day+1)
		if d := at[b.name] - at[a.name]; d == 1 || d == -1 {
			t.Errorf("day %d is %s and day %d is %s, which sit next to each other "+
				"in the list", day, a.name, day+1, b.name)
		}
	}
}

// Two rules hold for every prompt, whatever it is a picture of.
//
// Calm, because this lands on the home screen; and no text, because a model
// asked for a spiral or a contour map letters it — axis labels, a caption, a
// signature — and a picture with words in it says something nobody wrote.
func TestEveryPromptIsCalmAndWordless(t *testing.T) {
	// The words that make a prompt calm. One is enough; they are synonyms, not
	// a checklist.
	calm := []string{"calm", "quiet", "still", "serene", "peaceful", "gentle",
		"soft", "restful", "silent", "hushed", "slow", "unhurried", "meditative",
		"spare", "minimal", "warm"}

	for _, th := range dailyThemes {
		if th.name == "" || th.prompt == "" {
			t.Errorf("a theme is missing its name or prompt: %+v", th)
			continue
		}
		if !strings.Contains(strings.ToLower(th.prompt), "no text") {
			t.Errorf("%s does not tell the model to leave the words out", th.name)
		}
		lower := strings.ToLower(th.prompt)
		found := false
		for _, w := range calm {
			if strings.Contains(lower, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s does not ask for anything calm: %q", th.name, th.prompt)
		}
	}

	// And each is its own theme. A duplicate name shows twice on the archive
	// page under one label.
	seen := map[string]bool{}
	for _, th := range dailyThemes {
		if seen[th.name] {
			t.Errorf("two themes are called %s", th.name)
		}
		seen[th.name] = true
	}
}

// A geometric prompt says no numbers as well as no text.
//
// "No text" is not enough for a chart-shaped subject: asked for a contour field
// or an interference pattern, a model produces the diagram it has seen most,
// which has axes with numbers on them.
func TestGeometricPromptsRefuseLabels(t *testing.T) {
	for _, name := range []string{"fractals", "spirals", "waves", "contours",
		"tessellation", "voronoi", "flow"} {
		var prompt string
		for _, th := range dailyThemes {
			if th.name == name {
				prompt = strings.ToLower(th.prompt)
			}
		}
		if prompt == "" {
			t.Errorf("no theme called %s", name)
			continue
		}
		if !strings.Contains(prompt, "no labels") || !strings.Contains(prompt, "no numbers") {
			t.Errorf("%s is a diagram-shaped subject and does not refuse labels "+
				"or numbers: %q", name, prompt)
		}
	}
}
