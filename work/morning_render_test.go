package work

import (
	"mu/agent"
	"mu/service/events"
	"strings"
	"testing"
	"time"
)

func TestBriefRendererOwnsStructure(t *testing.T) {
	off := false
	e := &events.Event{Zone: "Europe/London", WorldNews: &off}
	c := agent.BriefContent{Day: []agent.BriefItem{{Text: "Meeting\n## Surprise", URL: "javascript:alert(1)"}}, Headlines: []agent.BriefItem{{Text: "Hidden news"}}, Priorities: []agent.BriefItem{{Text: "Hidden plan"}}}
	result := renderMorningBrief(c, "Asim", e, time.Date(2026, 9, 27, 23, 30, 0, 0, time.UTC), "", false)
	if strings.Contains(result, "\n## Surprise") || strings.Contains(result, "javascript:") || strings.Contains(result, "Hidden") {
		t.Fatal(result)
	}
	if !strings.Contains(result, "Monday, 28 September 2026") {
		t.Fatal("timezone lost", result)
	}
	previous := -1
	for _, heading := range []string{"## Your day", "## Weather", "## Prayer times", "## Daily reminder"} {
		position := strings.Index(result, heading)
		if position <= previous {
			t.Fatal(result)
		}
		previous = position
	}
}
