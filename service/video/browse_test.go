package video

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrowseIsBoundedAndHasReadingActions(t *testing.T) {
	all := map[string]Channel{}
	ch := Channel{}
	for i := 0; i < 14; i++ {
		ch.Videos = append(ch.Videos, &Result{ID: fmt.Sprintf("v%d", i), Title: fmt.Sprintf("Video %d", i), Category: "Tech", Channel: "The channel", ChannelID: "UCexample", Published: time.Now().Add(-time.Duration(i) * time.Minute)})
	}
	all["Tech"] = ch
	all["World"] = ch
	body := browse(httptest.NewRequest("GET", "/video?category=Tech&page=2", nil), all)
	if !strings.Contains(body, `id="video-search"`) || !strings.Contains(body, `id="recent-searches-container"`) {
		t.Fatal("missing search controls")
	}
	if !strings.Contains(body, `href="https://www.youtube.com/channel/UCexample"`) {
		t.Fatal("channel name is no longer linked to YouTube")
	}
	if strings.Count(body, `<article `) != 5 || !strings.Contains(body, `video-grid`) {
		t.Fatal("wrong video page")
	}
	if !strings.Contains(body, `/agent/micro?item=video_v9`) {
		t.Fatal("wrong archive reference")
	}
}
func TestWatchPageKeepsNavigationAndPlayerControls(t *testing.T) {
	indexVideo(&Result{ID: "example", Title: "A fetched video", Description: "The description", Channel: "The channel", ChannelID: "UCexample"})
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/video?id=example", nil))
	body := w.Body.String()
	for _, want := range []string{`href="/video">← Video</a>`, `/agent/micro?item=video_example`, `href="https://www.youtube.com/channel/UCexample"`, "Save", "Original", `id="audioBtn"`, `<body class="video-player-body">`, "allowfullscreen"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "%!s") || strings.Contains(body, "%!;") {
		t.Fatal("broken template formatting")
	}
}

func TestBrowseKeepsLegacyVideoCache(t *testing.T) {
	mutex.Lock()
	old, oldHTML := videos, videosHtml
	videos = map[string]Channel{}
	videosHtml = "legacy video"
	mutex.Unlock()
	defer func() { mutex.Lock(); videos, videosHtml = old, oldHTML; mutex.Unlock() }()
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/video", nil))
	if !strings.Contains(w.Body.String(), "legacy video") {
		t.Fatal("legacy cache lost")
	}
}

func TestUnknownWatchVideoDoesNotOfferBrokenReferences(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/video?id=unknown-video", nil))
	if strings.Contains(w.Body.String(), "/agent/micro?item=video_unknown-video") {
		t.Fatal("offered an unresolved attachment")
	}
}

func TestListPreservesEmptyFeedDiagnostic(t *testing.T) {
	mutex.Lock()
	old := videos
	videos = nil
	mutex.Unlock()
	defer func() { mutex.Lock(); videos = old; mutex.Unlock() }()
	var rsp ListResponse
	if err := (Server{}).List(context.Background(), &ListRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Text != LatestText(0) || rsp.Text == "" {
		t.Fatalf("lost diagnostic: %q", rsp.Text)
	}
}

func TestOverviewKeepsSlowChannelsVisible(t *testing.T) {
	now := time.Now()
	all := map[string]Channel{"Tech": {Videos: []*Result{
		{ID: "fast1", ChannelID: "fast", Title: "Fast newest", Published: now},
		{ID: "fast2", Title: "Fast older", Published: now.Add(-time.Hour)},
	}}, "Slow": {Videos: []*Result{
		{ID: "slow", Title: "Slow channel", Published: now.Add(-48 * time.Hour)},
	}}}
	body := browse(httptest.NewRequest("GET", "/video", nil), all)
	if strings.Count(body, "<article ") != 2 || !strings.Contains(body, "Slow channel") || strings.Contains(body, "Fast older") {
		t.Fatal("overview crowded out a channel")
	}
}
