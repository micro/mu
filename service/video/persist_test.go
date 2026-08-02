package video

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// What is persisted is data, not markup.
//
// videos.json used to carry each item's rendered HTML. Markup then outlived the
// code that wrote it: when thumbnails moved to Mu's own origin every cached
// item kept serving Google's URLs until its channel refetched an hour later,
// and the fix had to be a regex rewriting stored HTML on the way out. Anything
// derived from the fields beside it should be rendered, not stored.
func TestPersistedVideosCarryNoMarkup(t *testing.T) {
	ch := Channel{
		Videos: []*Result{{
			ID: "dQw4w9WgXcQ", Title: "A video", URL: "/video?id=dQw4w9WgXcQ",
			Channel: "Someone", ChannelID: "UC123", Category: "tech",
			Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg",
			Published: time.Now(),
		}},
	}
	ch.Html = renderChannel(ch)
	if ch.Html == "" {
		t.Fatal("renderChannel produced nothing")
	}

	b, err := json.Marshal(map[string]Channel{"tech": ch})
	if err != nil {
		t.Fatal(err)
	}
	stored := string(b)

	// "thumbnail" itself is data — the source URL YouTube gave us. What must
	// not be here is markup built from it.
	for _, markup := range []string{"<div", "<img", "<a href", "class="} {
		if strings.Contains(stored, markup) {
			t.Errorf("videos.json carries markup (%q): %s", markup, stored)
		}
	}
	if strings.Contains(stored, `"html"`) {
		t.Error("videos.json still has an html field")
	}
}

// A video loaded from disk with no markup renders against the current code —
// including the thumbnail path, which is the change that exposed this.
func TestVideosLoadedFromDiskRenderWithCurrentMarkup(t *testing.T) {
	var loaded map[string]Channel
	stored := `{"tech":{"videos":[{"id":"dQw4w9WgXcQ","title":"A video","url":"/video?id=dQw4w9WgXcQ",` +
		`"channel":"Someone","channel_id":"UC123","category":"tech",` +
		`"thumbnail":"https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg","published":"2026-01-01T00:00:00Z"}]}}`
	if err := json.Unmarshal([]byte(stored), &loaded); err != nil {
		t.Fatal(err)
	}

	html := renderChannel(loaded["tech"])
	if strings.Contains(html, "i.ytimg.com") {
		t.Errorf("a video loaded from disk still links to Google's CDN: %s", html)
	}
	for _, want := range []string{"/video/thumb?id=dQw4w9WgXcQ", "A video", "Someone"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered markup is missing %q: %s", want, html)
		}
	}
}
