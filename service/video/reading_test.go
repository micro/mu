package video

import (
	"context"
	"encoding/json"
	"fmt"
	"mu/internal/data"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
	"mu/internal/bookmarks"
)

func TestFetchedVideosCanBeSavedOutsideThePresetFeed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snippet := map[string]any{"title": "Fetched video", "description": "A fetched description", "channelTitle": "Publisher", "channelId": "publisher", "publishedAt": "2026-09-01T12:00:00Z"}
		var item map[string]any
		switch strings.TrimPrefix(r.URL.Path, "/youtube/v3") {
		case "/search":
			item = map[string]any{"id": map[string]any{"kind": "youtube#video", "videoId": "from-search"}, "snippet": snippet}
		case "/playlists":
			item = map[string]any{"snippet": snippet}
		case "/channels":
			item = map[string]any{"snippet": snippet, "contentDetails": map[string]any{"relatedPlaylists": map[string]any{"uploads": "uploads"}}}
		case "/playlistItems":
			id := "from-playlist"
			if r.URL.Query().Get("playlistId") == "uploads" {
				id = "from-channel"
			}
			snippet["resourceId"] = map[string]any{"videoId": id}
			item = map[string]any{"snippet": snippet}
		default:
			t.Errorf("unexpected upstream path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []any{item}})
	}))
	defer upstream.Close()
	clientOnce.Do(func() {})
	was := client
	defer func() { client = was }()
	var err error
	client, err = youtube.NewService(context.Background(), option.WithHTTPClient(upstream.Client()), option.WithEndpoint(upstream.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	wasBackend := data.UseSQLite
	defer func() { data.UseSQLite = wasBackend }()
	for _, sqlite := range []bool{true, false} {
		data.UseSQLite = sqlite
		t.Run(fmt.Sprintf("sqlite=%v", sqlite), func(t *testing.T) {
			html, _, err := getResults("topic", "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, `href="https://www.youtube.com/channel/publisher"`) {
				t.Fatal("search result channel does not link to YouTube")
			}
			for _, target := range []string{"/video?playlist=playlist", "/video?channel=publisher"} {
				w := httptest.NewRecorder()
				Handler(w, httptest.NewRequest("GET", target, nil))
				if w.Code != 200 {
					t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
				}
			}
			owner := "video-reading-fixture"
			defer bookmarks.Clear(owner)
			for _, id := range []string{"from-search", "from-playlist", "from-channel"} {
				item, err := bookmarks.Add(owner, bookmarks.Item{Ref: "video_" + id})
				if err != nil {
					t.Fatalf("cannot save %s: %v", id, err)
				}
				if item.Title != "Fetched video" || item.Excerpt != "A fetched description" {
					t.Fatalf("missing video metadata: %+v", item)
				}
			}
		})
	}
}
