package prayer

import (
	"strings"
	"testing"
)

func TestPrayerFiltersLeadWithVerseAndReflection(t *testing.T) {
	rd := &ReminderData{Verse: "verse-content", Hadith: "saying-content", Name: "name-content", Message: "reflection-content", Updated: "2026-01-01T00:00:00Z"}
	for _, view := range []string{"", "unknown", "verse"} {
		page := renderPrayerPage(rd, view)
		verse, reflection := strings.Index(page, "verse-content"), strings.Index(page, "reflection-content")
		if verse < 0 || reflection < verse {
			t.Error("default page must lead with verse then reflection")
		}
		if strings.Contains(page, `id="prayer-card"`) || strings.Contains(page, "saying-content") || strings.Contains(page, "name-content") {
			t.Error("unselected content shown")
		}
	}
	for _, tc := range []struct{ view, text string }{{"saying", "saying-content"}, {"name", "name-content"}, {"reflection", "reflection-content"}} {
		page := renderPrayerPage(rd, tc.view)
		if !strings.Contains(page, tc.text) || strings.Contains(page, "verse-content") {
			t.Errorf("wrong content for %s", tc.view)
		}
	}
	if page := renderPrayerPage(nil, "times"); !strings.Contains(page, `id="prayer-card"`) {
		t.Error("times should work without reminder data")
	}
	if page := renderPrayerPage(nil, ""); !strings.Contains(page, "not available") {
		t.Error("missing empty state")
	}
}

func TestPrayerReadingEscapesContent(t *testing.T) {
	page := renderPrayerPage(&ReminderData{Verse: `<script>bad()</script>`, Message: `<img onerror="bad()">`}, "")
	if strings.Contains(page, "<script>bad") || strings.Contains(page, "<img onerror") {
		t.Fatal("untrusted reminder markup rendered")
	}
}
