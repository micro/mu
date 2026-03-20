package video

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestResult_Structure(t *testing.T) {
	r := &Result{
		ID:          "abc123",
		Type:        "video",
		Title:       "Test Video",
		Description: "A test video description",
		URL:         "https://youtube.com/watch?v=abc123",
		Published:   time.Now(),
		Channel:     "TestChannel",
		ChannelID:   "UC123",
		Category:    "tech",
		Thumbnail:   "https://img.youtube.com/vi/abc123/mqdefault.jpg",
	}
	if r.ID != "abc123" {
		t.Error("expected ID")
	}
	if r.Channel != "TestChannel" {
		t.Error("expected channel")
	}
	if r.Category != "tech" {
		t.Error("expected category")
	}
}

func TestChannel_Structure(t *testing.T) {
	ch := Channel{
		Videos: []*Result{
			{ID: "1", Title: "Video 1"},
			{ID: "2", Title: "Video 2"},
		},
	}
	if len(ch.Videos) != 2 {
		t.Errorf("expected 2 videos, got %d", len(ch.Videos))
	}
}

func TestEmbedVideo(t *testing.T) {
	html := embedVideo("abc123")
	if !strings.Contains(html, "youtube.com/embed/abc123") {
		t.Error("expected YouTube embed URL")
	}
	if !strings.Contains(html, "iframe") {
		t.Error("expected iframe tag")
	}
	if strings.Contains(html, "autoplay=1") {
		t.Error("embedVideo should not autoplay")
	}
}

func TestEmbedVideoWithAutoplay(t *testing.T) {
	html := embedVideoWithAutoplay("xyz789", true)
	if !strings.Contains(html, "youtube.com/embed/xyz789") {
		t.Error("expected YouTube embed URL")
	}
	if !strings.Contains(html, "autoplay=1") {
		t.Error("expected autoplay parameter")
	}
}

func TestEmbedVideoWithAutoplay_NoAutoplay(t *testing.T) {
	html := embedVideoWithAutoplay("xyz789", false)
	if strings.Contains(html, "autoplay=1") {
		t.Error("should not have autoplay when false")
	}
}

func TestGetLatestVideos_Empty(t *testing.T) {
	mutex.Lock()
	origVideos := videos
	videos = map[string]Channel{}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		videos = origVideos
		mutex.Unlock()
	}()

	result := GetLatestVideos(5)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestGetLatestVideos_SortedByPublishDate(t *testing.T) {
	now := time.Now()
	mutex.Lock()
	origVideos := videos
	videos = map[string]Channel{
		"chan1": {
			Videos: []*Result{
				{ID: "old", Title: "Old", Published: now.Add(-24 * time.Hour)},
				{ID: "new", Title: "New", Published: now},
			},
		},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		videos = origVideos
		mutex.Unlock()
	}()

	result := GetLatestVideos(10)
	if len(result) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(result))
	}
	if result[0].ID != "new" {
		t.Errorf("expected newest first, got %q", result[0].ID)
	}
}

func TestGetLatestVideos_LimitsResults(t *testing.T) {
	now := time.Now()
	mutex.Lock()
	origVideos := videos
	videos = map[string]Channel{
		"chan1": {
			Videos: []*Result{
				{ID: "1", Published: now.Add(-3 * time.Hour)},
				{ID: "2", Published: now.Add(-2 * time.Hour)},
				{ID: "3", Published: now.Add(-1 * time.Hour)},
			},
		},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		videos = origVideos
		mutex.Unlock()
	}()

	result := GetLatestVideos(2)
	if len(result) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(result))
	}
}

func TestGetLatestVideos_AcrossChannels(t *testing.T) {
	now := time.Now()
	mutex.Lock()
	origVideos := videos
	videos = map[string]Channel{
		"chan1": {Videos: []*Result{{ID: "a", Published: now.Add(-2 * time.Hour)}}},
		"chan2": {Videos: []*Result{{ID: "b", Published: now}}},
	}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		videos = origVideos
		mutex.Unlock()
	}()

	result := GetLatestVideos(10)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].ID != "b" {
		t.Errorf("expected newest (b) first, got %q", result[0].ID)
	}
}

func TestVideosSortedByPublished(t *testing.T) {
	now := time.Now()
	results := []*Result{
		{ID: "1", Published: now.Add(-3 * time.Hour)},
		{ID: "2", Published: now},
		{ID: "3", Published: now.Add(-1 * time.Hour)},
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Published.After(results[j].Published)
	})
	if results[0].ID != "2" {
		t.Error("expected newest first")
	}
	if results[2].ID != "1" {
		t.Error("expected oldest last")
	}
}
