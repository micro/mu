// Daily image archive.
//
// The daily image used to live in a single JSON record that each morning's
// generation overwrote, so yesterday's image was gone the moment today's
// arrived. Worse, the only thing ever stored was the provider's URL — Mu never
// held the image itself, so the home card and every gallery entry pointed at a
// third-party CDN that can expire or disappear at any time.
//
// This keeps a real archive: the bytes are downloaded and stored locally, and
// each day is appended to a list rather than replacing it, so /images can show
// the run of past dailies and they keep working regardless of the provider.
package images

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
)

const (
	archiveKey = "images/archive.json"
	// archiveMax bounds the archive at roughly two years of dailies. Old
	// entries fall off the end and their stored bytes are removed with them.
	archiveMax = 730
	// maxImageBytes caps a single download. Dailies run well under this; the
	// limit is here so a misbehaving provider cannot fill the disk.
	maxImageBytes = 12 << 20 // 12 MiB
)

var (
	archiveMu sync.RWMutex
	archive   []Daily // newest first
)

// loadArchive restores the stored archive. Called from Load.
func loadArchive() {
	var a []Daily
	if err := data.LoadJSON(archiveKey, &a); err == nil {
		archiveMu.Lock()
		archive = a
		archiveMu.Unlock()
	}
}

// backfill adds a pre-archive daily to the archive, downloading its bytes if
// they were never stored. A no-op once the day is archived with a local file,
// so it is safe to call on every start.
func backfill(d Daily) {
	if d.Date == "" || d.URL == "" {
		return
	}
	for _, e := range Archive(0) {
		if e.Date == d.Date && e.File != "" {
			return
		}
	}
	if d.File == "" {
		d.File = storeImage(d.URL, d.Date)
	}
	archiveDaily(d)

	// Keep the in-memory daily in step so the home card serves the local copy.
	dailyMu.Lock()
	if daily.Date == d.Date {
		daily.File = d.File
	}
	dailyMu.Unlock()
	if err := data.SaveJSON(dailyKey, d); err != nil {
		app.Log("images", "failed to persist backfilled daily: %v", err)
	}
}

// Archive returns the stored dailies, newest first. limit <= 0 returns all.
func Archive(limit int) []Daily {
	archiveMu.RLock()
	defer archiveMu.RUnlock()

	if limit <= 0 || limit > len(archive) {
		limit = len(archive)
	}
	out := make([]Daily, limit)
	copy(out, archive[:limit])
	return out
}

// pastDailies returns archived dailies excluding the given date (normally
// today, shown separately as the hero), newest first.
func pastDailies(excludeDate string, limit int) []Daily {
	var out []Daily
	for _, d := range Archive(0) {
		if d.Date == excludeDate || d.URL == "" {
			continue
		}
		out = append(out, d)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// archiveDaily stores d as the entry for its date, replacing any existing entry
// for that day. Returns the trimmed-off entries so their files can be removed.
func archiveDaily(d Daily) (dropped []Daily) {
	archiveMu.Lock()
	defer archiveMu.Unlock()

	filtered := archive[:0:0]
	for _, e := range archive {
		if e.Date != d.Date {
			filtered = append(filtered, e)
		}
	}
	archive = append([]Daily{d}, filtered...)
	if len(archive) > archiveMax {
		dropped = append(dropped, archive[archiveMax:]...)
		archive = archive[:archiveMax]
	}
	snapshot := make([]Daily, len(archive))
	copy(snapshot, archive)

	if err := data.SaveJSON(archiveKey, snapshot); err != nil {
		app.Log("images", "failed to persist daily archive: %v", err)
	}
	return dropped
}

// storeImage downloads url and saves the bytes under images/daily/<date>.<ext>,
// returning the key. An empty key means the image could not be stored and the
// caller should keep using the remote URL.
func storeImage(url, date string) string {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		app.Log("images", "daily image download failed: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		app.Log("images", "daily image download returned %s", resp.Status)
		return ""
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		app.Log("images", "daily image read failed: %v", err)
		return ""
	}
	if len(b) == 0 || len(b) > maxImageBytes {
		app.Log("images", "daily image rejected: %d bytes", len(b))
		return ""
	}

	ext := imageExt(resp.Header.Get("Content-Type"), url)
	key := "images/daily/" + date + ext
	if err := data.SaveFile(key, string(b)); err != nil {
		app.Log("images", "failed to store daily image: %v", err)
		return ""
	}
	return key
}

// imageExt picks a file extension from the response content type, falling back
// to the URL path and finally to .png.
func imageExt(contentType, url string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	switch strings.ToLower(path.Ext(strings.SplitN(url, "?", 2)[0])) {
	case ".png":
		return ".png"
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".webp":
		return ".webp"
	case ".gif":
		return ".gif"
	}
	return ".png"
}

// contentTypeFor maps a stored key back to a content type for serving.
func contentTypeFor(key string) string {
	switch strings.ToLower(path.Ext(key)) {
	case ".jpg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

// validDate matches the YYYY-MM-DD date that names an archived image. The date
// comes from the request path and is used to build a storage key, so it is
// checked rather than escaped — anything else is not a date.
func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// DailyImageHandler serves an archived daily image at /images/daily/<date>.
// Images are immutable once archived, so they are cached hard.
func DailyImageHandler(w http.ResponseWriter, r *http.Request) {
	date := strings.Trim(strings.TrimPrefix(r.URL.Path, "/images/daily"), "/")
	if !validDate(date) {
		http.NotFound(w, r)
		return
	}

	var key string
	for _, d := range Archive(0) {
		if d.Date == date && d.File != "" {
			key = d.File
			break
		}
	}
	if key == "" {
		http.NotFound(w, r)
		return
	}

	b, err := data.LoadFile(key)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentTypeFor(key))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
	w.Write(b)
}

// displayURL returns the URL to render for a daily: the locally stored copy
// when we have one, otherwise the provider URL we started with.
func (d Daily) displayURL() string {
	if d.File != "" {
		return "/images/daily/" + d.Date
	}
	return d.URL
}
