package prayer

import (
	"strings"
	"testing"
	"time"
)

// One key per reflection, and reflections arrive hourly.
//
// Keying on the date would collapse twenty-four of them into whichever came
// last — the same bug as the constant id "daily", with a longer period.
func TestEachHourlyReflectionGetsItsOwnKey(t *testing.T) {
	morning := reflectionKey("2026-03-14T09:00:00Z")
	later := reflectionKey("2026-03-14T10:00:00Z")
	if morning == later {
		t.Errorf("two reflections an hour apart share the key %q", morning)
	}
	if !strings.HasPrefix(morning, "2026-03-14T09") {
		t.Errorf("the key %q does not carry the hour it was published", morning)
	}
}

// The same publication is the same key, whatever offset it arrived in and
// however many times it is fetched. Keying on the fetch time would write a row
// an hour for a reflection that had not changed.
func TestTheSamePublicationIsOneEntry(t *testing.T) {
	utc := reflectionKey("2026-03-14T09:00:00Z")
	if got := reflectionKey("2026-03-14T09:00:00Z"); got != utc {
		t.Error("the same stamp gave two keys")
	}
	// The live feed stamps with an offset — 2026-08-30T12:14:08+01:00.
	if got := reflectionKey("2026-03-14T10:00:00+01:00"); got != utc {
		t.Errorf("the same moment in another offset gave %q, want %q", got, utc)
	}
	for _, layout := range []string{"2026-03-14 09:00:00"} {
		if got := reflectionKey(layout); !strings.HasPrefix(got, "2026-03-14T09") {
			t.Errorf("reflectionKey(%q) = %q", layout, got)
		}
	}
}

// An unreadable stamp still gets filed, keyed to the hour we saw it — worse,
// and better than losing the reflection.
func TestAnUnreadableStampStillGetsFiled(t *testing.T) {
	hour := time.Now().UTC().Format("2006-01-02T15Z")
	for _, in := range []string{"", "   ", "yesterday", "not a date"} {
		if got := reflectionKey(in); got != hour {
			t.Errorf("reflectionKey(%q) = %q, want the current hour (%s)", in, got, hour)
		}
	}
}

// The archive list shows the verse, not a column of timestamps.
func TestTheTitleNamesTheVerse(t *testing.T) {
	got := reflectionTitle(map[string]interface{}{
		"verse": "Ya-Sin - Ya Sin - 36:13\nGive them an example of the residents of a town.",
	}, "2026-03-14T09:00:00Z")
	if !strings.Contains(got, "36:13") {
		t.Errorf("the title does not name the verse: %q", got)
	}
	if strings.Contains(got, "Give them an example") {
		t.Errorf("the title carries the whole verse: %q", got)
	}
	// With no verse it still says something findable.
	if got := reflectionTitle(map[string]interface{}{}, "2026-03-14T09:00:00Z"); got == "" {
		t.Error("a reflection with no verse got no title at all")
	}
}

// The publisher's own deep links are kept, so an entry leads back to the verse
// rather than to the site's front page.
func TestTheSourceLinksAreKept(t *testing.T) {
	meta := reflectionMeta(map[string]interface{}{
		"links": map[string]interface{}{
			"verse":  "/quran/36#13",
			"hadith": "/hadith/70#5427",
			"name":   "/names/76",
		},
	}, "2026-03-14T09:00:00Z")

	for k, want := range map[string]string{
		"verse_url":  "https://reminder.dev/quran/36#13",
		"hadith_url": "https://reminder.dev/hadith/70#5427",
		"name_url":   "https://reminder.dev/names/76",
	} {
		if got, _ := meta[k].(string); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	// And a payload with no links carries none rather than empty ones.
	bare := reflectionMeta(map[string]interface{}{}, "2026-03-14T09:00:00Z")
	for _, k := range []string{"verse_url", "hadith_url", "name_url"} {
		if _, present := bare[k]; present {
			t.Errorf("%s is present with nothing behind it", k)
		}
	}
}

// The whole reflection is kept, not the summary.
//
// This is the thing that was missing. Only "message" was indexed, so the verse,
// the hadith and the name — the parts somebody would go looking for — were
// dropped at index time and existed nowhere afterwards.
func TestTheWholeReflectionIsKept(t *testing.T) {
	got := reflectionText(map[string]interface{}{
		"verse":   "Indeed, with hardship comes ease — 94:6",
		"hadith":  "The best of you are those best to their families.",
		"name":    "Ar-Rahman — The Most Merciful",
		"message": "A reminder about patience.",
	})

	for _, want := range []string{
		"94:6", "best to their families", "Ar-Rahman", "patience",
		"Verse:", "Hadith:", "Name:", "Reflection:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the kept text is missing %q:\n%s", want, got)
		}
	}
}

// A reflection with parts missing keeps what there is, with no empty labels.
func TestMissingPartsAreNotLabelled(t *testing.T) {
	got := reflectionText(map[string]interface{}{
		"verse":   "Indeed, with hardship comes ease — 94:6",
		"hadith":  "",
		"message": "   ",
	})
	if !strings.Contains(got, "Verse:") {
		t.Error("the verse was dropped")
	}
	for _, gone := range []string{"Hadith:", "Name:", "Reflection:"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s is labelled with nothing under it:\n%s", gone, got)
		}
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("the text ends in whitespace")
	}
}
