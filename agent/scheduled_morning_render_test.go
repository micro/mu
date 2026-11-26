package agent

import (
	"mu/internal/app"
	"mu/service/events"
	"mu/service/markets"
	"mu/service/prayer"
	"strings"
	"testing"
	"time"
)

func TestBriefRendererOwnsStructure(t *testing.T) {
	off := false
	e := &events.Event{Zone: "Europe/London", WorldNews: &off}
	c := BriefContent{Day: []BriefItem{{Text: "Meeting\n## Surprise", URL: "javascript:alert(1)"}}, Headlines: []BriefItem{{Text: "Hidden news"}}, Priorities: []BriefItem{{Text: "Hidden plan"}}}
	result := renderMorningBrief(c, "Asim", e, time.Date(2026, 9, 27, 23, 30, 0, 0, time.UTC), "## Daily reminder\n\nA verse.\n\n# Spiritual Reflection\n\nReflection text.", false)
	if strings.Contains(result, "\n## Surprise") || strings.Contains(result, "javascript:") || strings.Contains(result, "Hidden") {
		t.Fatal(result)
	}
	if !strings.Contains(result, "Monday, 28 September 2026") {
		t.Fatal("timezone lost", result)
	}
	previous := -1
	for _, heading := range []string{"## Your day", "## Daily reminder", "## Spiritual Reflection"} {
		position := strings.Index(result, heading)
		if position <= previous {
			t.Fatal(result)
		}
		previous = position
	}
}

func TestMorningMarketsPreserveFreshness(t *testing.T) {
	now := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	items := briefMarketItems(map[string]markets.PriceData{
		"BTC":  {Price: 65000, Change24h: -2.5, UpdatedAt: now.Add(-time.Minute), Source: "provider"},
		"GOLD": {Price: 2500, UpdatedAt: now.Add(-24 * time.Hour)},
		"GBP":  {Price: 1.3},
		"ETH":  {Price: 2500, UpdatedAt: now.Add(time.Hour)},
	}, now)
	if len(items) != 2 || items[0].Title != "BTC" || items[1].Title != "GOLD" {
		t.Fatalf("unexpected market rows: %+v", items)
	}
	result := renderMorningBrief(BriefContent{Markets: items}, "", &events.Event{Zone: "UTC"}, now, "", false)
	for _, want := range []string{"- **BTC** — $65000.00 (-2.50%)", "- **GOLD** — $2500.00 (+0.00%) — as of 28 Sep, 06:00 UTC"} {
		if !strings.Contains(result, want) {
			t.Fatalf("missing %q: %s", want, result)
		}
	}
	if strings.Contains(items[0].Text, "as of") {
		t.Fatal("fresh quote marked as old")
	}
}

func TestMorningMarketsRenderEveryAssetInOrder(t *testing.T) {
	now := time.Now().UTC()
	symbols := []string{"BTC", "ETH", "GOLD", "OIL", "GBP", "EUR", "AAPL", "MSFT"}
	prices := map[string]markets.PriceData{}
	for _, symbol := range symbols {
		prices[symbol] = markets.PriceData{Price: 1.34604, Change24h: 0.38, UpdatedAt: now}
	}
	items := briefMarketItems(prices, now)
	result := renderMorningBrief(BriefContent{Markets: items}, "", &events.Event{Zone: "UTC"}, now, "", false)
	previous := -1
	for _, symbol := range symbols {
		needle := "\n- **" + symbol + "** — "
		position := strings.Index(result, needle)
		if position <= previous || strings.Count(result, needle) != 1 {
			t.Fatalf("missing, repeated or unordered %s: %s", symbol, result)
		}
		previous = position
	}
	if !strings.Contains(result, "- **GBP** — $1.34604 (+0.38%)") {
		t.Fatal("currency precision lost", result)
	}
	if len(briefMarketItems(nil, now)) != 0 {
		t.Fatal("invented market rows without quotes")
	}
}

func TestMorningTravelAppearsAfterWeather(t *testing.T) {
	content, err := parseBriefContent(`{"day":[{"text":"Meeting at 09:00"}],"travel":[{"text":"TfL: Central line delays","url":"https://tfl.gov.uk/"}]}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := renderMorningBrief(content, "", &events.Event{Zone: "UTC"}, time.Now(), "## Daily reminder\n\nA reminder.", false)
	travel := strings.Index(result, "## Travel")
	if travel < strings.Index(result, "## Your day") || travel > strings.Index(result, "## Daily reminder") || strings.Contains(result[:travel], "Central line") || !strings.Contains(result[travel:], "Central line delays") {
		t.Fatal(result)
	}
	empty := renderMorningBrief(BriefContent{}, "", &events.Event{Zone: "UTC"}, time.Now(), "", false)
	if strings.Contains(empty, "## Travel") {
		t.Fatal("empty travel section rendered")
	}
}

func TestMorningReminderQuotesDatedVerse(t *testing.T) {
	const date = "2026-10-19"
	d := &prayer.ReminderData{Date: date, Message: "Exact daily reflection.", Verse: "A long verse", Hadith: "A long hadith", Name: "A long name"}
	result := morningReminder(d, date)
	if !strings.Contains(result, d.Verse) || !strings.Contains(result, "/prayer?date="+date+")") {
		t.Fatal(result)
	}
	for _, unwanted := range []string{d.Message, d.Hadith, d.Name} {
		if strings.Contains(result, unwanted) {
			t.Fatal("brief lengthened", result)
		}
	}
	if morningReminder(d, "2026-10-20") != "" {
		t.Fatal("stale reminder accepted")
	}
}

func TestMorningBriefSeparatesContentFromSources(t *testing.T) {
	c := BriefContent{
		Day:       []BriefItem{{Text: "09:00 meeting", URL: "https://micro.mu/events"}, {Text: "14:00 appointment", URL: "https://micro.mu/events"}},
		Headlines: []BriefItem{{Title: "Rail service resumes", Text: "Repairs finished overnight.", URL: "https://example.com/rail"}},
	}
	result := renderMorningBrief(c, "", &events.Event{Zone: "UTC"}, time.Now(), "", false)
	if strings.Contains(result, "[Source]") || !strings.Contains(result, "- **Rail service resumes** — Repairs finished overnight.") {
		t.Fatal(result)
	}
	rendered := app.RenderString(result)
	if !strings.Contains(rendered, "<strong>Rail service resumes</strong>") || !strings.Contains(rendered, `href="https://example.com/rail"`) {
		t.Fatal("rendered hierarchy lost", rendered)
	}
	sources := strings.Index(result, "Sources:")
	if sources < strings.Index(result, "## Daily reminder") || strings.Contains(result[:sources], "https://") || strings.Count(result, "https://micro.mu/events") != 1 {
		t.Fatal("sources must be deduplicated and follow the content", result)
	}
}

func TestBriefOmitsEmptyOptionalSections(t *testing.T) {
	c := BriefContent{Weather: []BriefItem{{Text: "  "}}, Travel: []BriefItem{{Text: ""}}, Priorities: []BriefItem{{Text: ""}}}
	got := renderMorningBrief(c, "", &events.Event{Zone: "UTC"}, time.Now(), "", true)
	for _, heading := range []string{"## Weather", "## Travel", "## Prayer times", "## Headlines", "## Markets", "## One small step"} {
		if strings.Contains(got, heading) {
			t.Fatal("empty section", got)
		}
	}
	if !strings.Contains(got, "may not include all your commitments") || !strings.Contains(got, "Today's verse is unavailable") {
		t.Fatal("missing important availability notice", got)
	}
}

func TestMorningPrayerTimesAreIndividualBullets(t *testing.T) {
	times := &prayer.PrayerTimes{Fajr: "05:57", Sunrise: "07:30", Dhuhr: "11:49", Asr: "13:40", Maghrib: "16:00", Isha: "17:40"}
	items := briefPrayerItems(times)
	result := renderMorningBrief(BriefContent{Prayer: items}, "", &events.Event{Zone: "Europe/London"}, time.Now(), "", false)
	start := strings.Index(result, "## Prayer times")
	end := strings.Index(result, "## Daily reminder")
	section := result[start:end]
	for _, want := range []string{"- **Fajr** — 05:57", "- **Dhuhr** — 11:49", "- **Asr** — 13:40", "- **Maghrib** — 16:00", "- **Isha** — 17:40"} {
		if strings.Count(section, want) != 1 {
			t.Fatalf("missing or repeated %q: %s", want, section)
		}
	}
	if strings.Count(section, "\n- ") != 5 || strings.Contains(section, "Sunrise") || strings.Contains(section, "- **Prayer times**") {
		t.Fatal(section)
	}
	if !strings.Contains(section, "\n\nISNA calculation, not congregation times.\n") {
		t.Fatal("calculation note must be separate", section)
	}
	rendered := app.RenderString(section)
	if strings.Count(rendered, "<li>") != 5 || !strings.Contains(rendered, "<p>ISNA calculation, not congregation times.</p>") {
		t.Fatal("rendered prayer layout", rendered)
	}
	times.Fajr = ""
	if len(briefPrayerItems(times)) != 4 || len(briefPrayerItems(nil)) != 0 {
		t.Fatal("missing times were invented")
	}
}

func TestMorningOverviewPrecedesDetails(t *testing.T) {
	content, err := parseBriefContent(`{"overview":"Rail services have resumed after overnight repairs. Rain is expected before your afternoon appointment.","day":[{"text":"Appointment at 14:00"}],"prayer":[{"title":"Prayer times","text":"Invented times"}]}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Prayer) != 0 {
		t.Fatal("model prayer times must not be used")
	}
	result := renderMorningBrief(content, "Asim", &events.Event{Zone: "UTC"}, time.Now(), "", false)
	heading := strings.Index(result, "## Overview")
	day := strings.Index(result, "## Your day")
	if heading < 0 || heading > day || !strings.Contains(result, "## Overview\n\n"+content.Overview+"\n") {
		t.Fatal("missing overview paragraph", result)
	}
	content.Overview = "Text\n## Injected <script>"
	result = renderMorningBrief(content, "", &events.Event{Zone: "UTC"}, time.Now(), "", false)
	if strings.Contains(result, "\n## Injected") || strings.Contains(result, "<script>") {
		t.Fatal("overview injected structure", result)
	}
}
