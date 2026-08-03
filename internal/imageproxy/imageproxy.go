// Package imageproxy serves remote images from this instance.
//
// Article cards carry an image from the publisher's CDN — bbci.co.uk,
// cnbcfm.com, guim.co.uk. Embedding those URLs directly makes every page a
// request to four ad-tech CDNs on the reader's behalf, and hands each of them
// the decision about whether the page renders: hotlink rules, resource
// policies, a content blocker's filter list, an expiring signed URL, or a rate
// limit against a page carrying five hundred of them. When one says no the
// image is simply gone, and the `onerror` handler hides it, so the page looks
// like it never had a picture.
//
// The same argument the images service makes for a generated image applies
// here: if we are going to show it, we should hold it. Bytes are fetched once,
// server-side, cached in blob storage, and served from this origin — where
// `img-src 'self'` allows them, no third party sees the reader, and the
// publisher is asked once rather than once per visitor.
//
// It is not an open proxy. Only a URL this instance itself signed can be
// fetched, so nobody can point /img at an address of their choosing.
package imageproxy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/blob"
	"mu/internal/data"
	"mu/internal/safefetch"
	"mu/internal/settings"
)

const (
	// Path is the endpoint proxied images are served from.
	Path = "/img"

	// maxBytes caps one image. Article cover images run well under this.
	maxBytes = 8 << 20

	// fetchTimeout bounds a single upstream fetch.
	fetchTimeout = 20 * time.Second

	// maxConcurrent bounds outbound fetches. A page of link cards asks for
	// hundreds of images at once; without this, one page view becomes hundreds
	// of simultaneous connections and looks like an attack to the publisher.
	maxConcurrent = 8

	// negativeTTL is how long a failure is remembered. A page with five hundred
	// cards must not retry a dead host five hundred times per render.
	negativeTTL = 10 * time.Minute

	// keepFor is how long a cached image is kept. Article images stop being
	// referenced when the story rolls off the feed; this bounds the disk.
	keepFor = 30 * 24 * time.Hour

	// maxEntries caps the cache by count as well as by age.
	maxEntries = 20000

	indexKey = "imgcache/index.json"
)

// entry is one cached image: where its bytes are and what they are.
type entry struct {
	Key     string    `json:"key"`
	Type    string    `json:"type"`
	Fetched time.Time `json:"fetched"`
}

var (
	mu    sync.RWMutex
	index = map[string]entry{} // url hash -> entry

	failMu sync.Mutex
	failed = map[string]time.Time{} // url hash -> when it failed

	inflight sync.Map // url hash -> *sync.Once-ish gate
	slots    = make(chan struct{}, maxConcurrent)

	secretOnce sync.Once
	secret     []byte
)

// Load restores the cache index and starts the sweeper.
func Load() {
	var idx map[string]entry
	if err := data.LoadJSON(indexKey, &idx); err == nil && idx != nil {
		mu.Lock()
		index = idx
		mu.Unlock()
	}
	go sweeper()
}

// signingKey is stable across restarts when the instance has an encryption key,
// so a URL in a reader's cached page keeps working. Without one it is random
// per process: pages are rendered per request, so the only cost is that a page
// held open across a restart loses its images on reload.
func signingKey() []byte {
	secretOnce.Do(func() {
		if k := strings.TrimSpace(settings.Get("MU_ENCRYPTION_KEY")); k != "" {
			sum := sha256.Sum256([]byte("imageproxy:" + k))
			secret = sum[:]
			return
		}
		secret = make([]byte, 32)
		rand.Read(secret)
	})
	return secret
}

func sign(remote string) string {
	m := hmac.New(sha256.New, signingKey())
	m.Write([]byte(remote))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

func hashOf(remote string) string {
	sum := sha256.Sum256([]byte(remote))
	return hex.EncodeToString(sum[:])
}

// URL returns the local URL to render a remote image from, or the remote URL
// unchanged when it is not something to proxy (already ours, a data: URI, or
// not http(s) at all).
func URL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if !strings.HasPrefix(remote, "http://") && !strings.HasPrefix(remote, "https://") {
		return remote // relative, data:, blob: — already ours or not fetchable
	}
	return Path + "?u=" + url.QueryEscape(remote) + "&s=" + sign(remote)
}

// Handler serves a signed remote image from cache, fetching it once if this is
// the first time it has been asked for.
func Handler(w http.ResponseWriter, r *http.Request) {
	remote := r.URL.Query().Get("u")
	if remote == "" || !hmac.Equal([]byte(r.URL.Query().Get("s")), []byte(sign(remote))) {
		// Unsigned, or signed by something that is not this instance. Nothing
		// here is fetchable by asking.
		http.NotFound(w, r)
		return
	}

	e, ok := lookup(remote)
	if !ok {
		var err error
		e, err = fetch(r.Context(), remote)
		if err != nil {
			fallback(w, r, remote)
			return
		}
	}

	b, err := blob.Get(e.Key)
	if err != nil {
		// The index and the store disagree — drop the entry so the next request
		// fetches it again rather than failing forever.
		forget(remote)
		fallback(w, r, remote)
		return
	}

	w.Header().Set("Content-Type", e.Type)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Addressed by content, so it can be cached hard.
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Write(b)
}

// fallback sends the reader to the original image when we could not take a copy
// of it.
//
// This is what makes the proxy safe to turn on everywhere: some CDNs refuse a
// datacentre IP while allowing a home one, so a server-side fetch can fail
// where the reader's own browser would have succeeded. Falling back is exactly
// the old behaviour — a cross-origin embed that may or may not render — so the
// proxy can only improve on it, never take an image away.
//
// The URL is one this instance signed, checked before we get here, so this is
// not a redirector anyone can point somewhere.
func fallback(w http.ResponseWriter, r *http.Request, remote string) {
	if strings.HasPrefix(remote, "https://") {
		w.Header().Set("Cache-Control", "public, max-age=300")
		http.Redirect(w, r, remote, http.StatusFound)
		return
	}
	http.NotFound(w, r)
}

func lookup(remote string) (entry, bool) {
	mu.RLock()
	e, ok := index[hashOf(remote)]
	mu.RUnlock()
	return e, ok
}

func forget(remote string) {
	mu.Lock()
	delete(index, hashOf(remote))
	mu.Unlock()
	persist()
}

// fetch downloads an image and caches it. Concurrent requests for the same URL
// wait on the first rather than each making their own request upstream.
func fetch(ctx context.Context, remote string) (entry, error) {
	h := hashOf(remote)

	failMu.Lock()
	if when, ok := failed[h]; ok && time.Since(when) < negativeTTL {
		failMu.Unlock()
		return entry{}, fmt.Errorf("recently failed")
	}
	failMu.Unlock()

	gate, _ := inflight.LoadOrStore(h, &sync.Mutex{})
	lock := gate.(*sync.Mutex)
	lock.Lock()
	defer func() {
		lock.Unlock()
		inflight.Delete(h)
	}()

	// Another request may have finished while this one waited.
	if e, ok := lookup(remote); ok {
		return e, nil
	}

	slots <- struct{}{}
	defer func() { <-slots }()

	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
	defer cancel()

	body, ctype, err := fetchRemote(fetchCtx, remote)
	if err != nil {
		remember(h)
		app.Log("imageproxy", "fetch %s: %v", remote, err)
		return entry{}, fmt.Errorf("could not fetch")
	}
	if !isImage(ctype) {
		remember(h)
		app.Log("imageproxy", "fetch %s: not an image (%s)", remote, ctype)
		return entry{}, fmt.Errorf("not an image")
	}
	if len(body) == 0 {
		remember(h)
		return entry{}, fmt.Errorf("empty")
	}

	key := "imgcache/" + h
	if err := blob.Put(key, body, ctype); err != nil {
		app.Log("imageproxy", "store %s: %v", remote, err)
		return entry{}, err
	}

	e := entry{Key: key, Type: ctype, Fetched: time.Now().UTC()}
	mu.Lock()
	index[h] = e
	mu.Unlock()
	persist()
	return e, nil
}

// fetchRemote gets the bytes. A variable so a test can serve from a loopback
// address, which the real one refuses on purpose.
var fetchRemote = safeFetchImage

// safeFetchImage fetches through the SSRF guard: this takes a URL that came
// from a third-party page's metadata, so it must never be able to reach
// anything on the instance's own network.
func safeFetchImage(ctx context.Context, remote string) ([]byte, string, error) {
	rsp, err := safefetch.Fetch(ctx, remote, safefetch.Options{
		MaxBytes: maxBytes,
		Timeout:  fetchTimeout,
		Headers: map[string]string{
			// Ask the way a browser would. A bare Go client is one of the
			// things CDNs refuse.
			"Accept":     "image/avif,image/webp,image/apng,image/*,*/*;q=0.8",
			"User-Agent": "Mozilla/5.0 (compatible; Mu/1.0; +https://micro.mu)",
		},
	})
	if err != nil {
		return nil, "", err
	}
	if rsp.Status != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", rsp.Status)
	}
	return []byte(rsp.Body), contentType(rsp.Headers), nil
}

func remember(h string) {
	failMu.Lock()
	failed[h] = time.Now()
	failMu.Unlock()
}

func contentType(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "Content-Type") {
			return strings.ToLower(strings.TrimSpace(strings.SplitN(v, ";", 2)[0]))
		}
	}
	return ""
}

// isImage accepts raster images only. SVG is XML that can carry script, and
// this serves from Mu's own origin — an SVG here would run as Mu.
func isImage(ctype string) bool {
	if !strings.HasPrefix(ctype, "image/") {
		return false
	}
	return ctype != "image/svg+xml"
}

func persist() {
	mu.RLock()
	snapshot := make(map[string]entry, len(index))
	for k, v := range index {
		snapshot[k] = v
	}
	mu.RUnlock()
	if err := data.SaveJSON(indexKey, snapshot); err != nil {
		app.Log("imageproxy", "failed to persist the cache index: %v", err)
	}
}

// sweeper drops images nobody is asking for any more. Article images stop being
// referenced when the story rolls off the feed, and without this the cache is
// the one thing on a self-hosted instance that only ever grows.
func sweeper() {
	for {
		time.Sleep(6 * time.Hour)
		sweep()
	}
}

func sweep() {
	mu.Lock()
	var dropped []string // blob keys, collected as entries leave the index

	for h, e := range index {
		if time.Since(e.Fetched) > keepFor {
			dropped = append(dropped, e.Key)
			delete(index, h)
		}
	}
	// Still over the count cap: drop oldest first.
	for len(index) > maxEntries {
		oldest, oldestAt := "", time.Now().Add(time.Hour)
		for h, e := range index {
			if e.Fetched.Before(oldestAt) {
				oldest, oldestAt = h, e.Fetched
			}
		}
		if oldest == "" {
			break
		}
		dropped = append(dropped, index[oldest].Key)
		delete(index, oldest)
	}
	mu.Unlock()

	if len(dropped) == 0 {
		return
	}
	for _, k := range dropped {
		blob.Delete(k) //nolint:errcheck — a leftover blob is not worth failing over
	}
	persist()
	app.Log("imageproxy", "swept %d cached images", len(dropped))
}
