package video

import (
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
		ch.Videos = append(ch.Videos, &Result{ID: fmt.Sprintf("v%d", i), Title: fmt.Sprintf("Video %d", i), Category: "Tech", Published: time.Now().Add(-time.Duration(i) * time.Minute)})
	}
	all["Tech"] = ch
	all["World"] = ch
	body := browse(httptest.NewRequest("GET", "/video?category=Tech&page=2", nil), all)
	if strings.Count(body, `<article `) != 2 || !strings.Contains(body, `video-grid`) {
		t.Fatal("wrong video page")
	}
	if !strings.Contains(body, `/agent/micro?item=video_v12`) {
		t.Fatal("wrong archive reference")
	}
}
func TestWatchPageKeepsNavigationAndPlayerControls(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/video?id=example", nil))
	body := w.Body.String()
	for _, want := range []string{"← Back to video", "Ask Micro", "Save", "Original", "toggleAudio()", "allowfullscreen"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "%!s") || strings.Contains(body, "%!;") {
		t.Fatal("broken template formatting")
	}
}
