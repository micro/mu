package video

import (
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"mu/internal/app"
)

// Thumbnails are served from Mu rather than linked straight to Google's CDN.
//
// Two reasons, and the second is the one that matters.
//
// It works. A page of videos is two hundred images from one hostname,
// i.ytimg.com, and anything that blocks that hostname — a content blocker, a
// filtering resolver, a network policy — breaks every thumbnail on the page at
// once. Served from Mu's own origin they are as reachable as the page itself.
//
// It does not leak. Linking directly meant every visitor's browser announced
// itself to Google — IP address, and which videos they were looking at — on
// every page load, before they clicked anything. Mu's whole claim is that the
// everyday internet can work without that, and /video promises YouTube without
// the tracking. Two hundred requests to Google's CDN is not that promise.

// videoID is YouTube's identifier format. The proxy builds its own upstream URL
// from a matching id and nothing else, so there is no caller-supplied URL to
// point somewhere it should not go.
var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

const (
	thumbTTL      = 24 * time.Hour
	thumbMaxBytes = 512 << 10 // a mqdefault is ~10KB; this is a wide ceiling
	thumbMaxItems = 600
)

type thumb struct {
	body    []byte
	kind    string
	fetched time.Time
}

var (
	thumbMu    sync.RWMutex
	thumbCache = map[string]thumb{}
)

// ThumbURL is the local path for a video's thumbnail.
func ThumbURL(id string) string {
	if !videoID.MatchString(id) {
		return ""
	}
	return "/video/thumb?id=" + id
}

// ThumbHandler serves a cached thumbnail from Mu's own origin.
func ThumbHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !videoID.MatchString(id) {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	if t, ok := cachedThumb(id); ok {
		writeThumb(w, t)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		"https://i.ytimg.com/vi/"+id+"/mqdefault.jpg", nil)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadGateway)
		return
	}
	rsp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(rsp.Body, thumbMaxBytes))
	if err != nil || len(body) == 0 {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	kind := rsp.Header.Get("Content-Type")
	if kind == "" {
		kind = "image/jpeg"
	}

	t := thumb{body: body, kind: kind, fetched: time.Now()}
	storeThumb(id, t)
	writeThumb(w, t)
}

func writeThumb(w http.ResponseWriter, t thumb) {
	w.Header().Set("Content-Type", t.kind)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(t.body)
}

func cachedThumb(id string) (thumb, bool) {
	thumbMu.RLock()
	t, ok := thumbCache[id]
	thumbMu.RUnlock()
	if !ok || time.Since(t.fetched) > thumbTTL {
		return thumb{}, false
	}
	return t, true
}

// storeThumb keeps the cache bounded. Thumbnails are small and a feed reuses
// the same ones, so dropping the whole map when it fills is enough — an LRU
// would cost more than it saves here.
func storeThumb(id string, t thumb) {
	thumbMu.Lock()
	defer thumbMu.Unlock()
	if len(thumbCache) >= thumbMaxItems {
		thumbCache = map[string]thumb{}
		app.Log("video", "thumbnail cache full (%d), cleared", thumbMaxItems)
	}
	thumbCache[id] = t
}
