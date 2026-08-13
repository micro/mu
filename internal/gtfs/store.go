package gtfs

// Keeping feeds current without ever being without one.
//
// The rule this is built around: nothing already working is removed until a
// replacement has been built and opened successfully. A refresh downloads to a
// temporary file, builds into a temporary directory, opens the result, and only
// then swaps — so a truncated download, a corrupt zip or an agency publishing
// nonsense costs a log line and yesterday's timetable stays answerable.
//
// # How often
//
// Agencies republish when the timetable changes, which is a few times a year
// for a small operator and weekly for a big one. Nobody publishes hourly, so
// polling hourly would be rude and pointless. This checks once a day, and the
// check is a conditional GET: If-None-Match and If-Modified-Since, so an
// unchanged feed costs a 304 and no bytes. Berlin's 75MB is only fetched when
// Berlin has actually changed.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RefreshInterval is how often a feed is asked whether it has changed.
const RefreshInterval = 24 * time.Hour

// maxFeedBytes bounds a download. Berlin, the largest feed in the catalogue by
// some distance, is 75MB; this leaves room to grow without letting a
// misconfigured URL fill the disk.
const maxFeedBytes = 400 << 20

var client = &http.Client{Timeout: 10 * time.Minute}

// Store holds the feeds this instance serves.
type Store struct {
	dir string

	mu    sync.RWMutex
	feeds map[string]*Index
}

// NewStore opens whatever has already been built under dir.
func NewStore(dir string) *Store {
	s := &Store{dir: dir, feeds: map[string]*Index{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return s
	}
	for _, e := range entries {
		// Leftovers from an interrupted refresh are not feeds.
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.Contains(e.Name(), ".tmp") {
			continue
		}
		idx, err := Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		s.feeds[e.Name()] = idx
	}
	return s
}

// Loaded names the feeds currently answerable.
func (s *Store) Loaded() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.feeds))
	for id := range s.feeds {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Get returns a loaded feed.
func (s *Store) Get(id string) (*Index, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.feeds[id]
	return i, ok
}

// All returns every loaded feed.
func (s *Store) All() []*Index {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Index, 0, len(s.feeds))
	for _, i := range s.feeds {
		out = append(out, i)
	}
	return out
}

// NearestFeed picks the feed whose stops are closest to a point.
//
// Feeds overlap — a city has an operator per mode and a regional feed over the
// top — so the question "which feed covers here" has no clean answer. Nearest
// stop is a blunt one that behaves well: the feed with a stop on this street
// is the feed that knows about this street.
func (s *Store) NearestFeed(lat, lon float64) (*Index, float64) {
	var best *Index
	bestKm := 1e18
	for _, idx := range s.All() {
		near := idx.Near(lat, lon, 1)
		if len(near) == 0 {
			continue
		}
		if d := DistanceKm(lat, lon, near[0].Lat, near[0].Lon); d < bestKm {
			best, bestKm = idx, d
		}
	}
	return best, bestKm
}

// Refresh fetches a feed if it has changed, and swaps it in if it builds.
//
// urls are tried in order and the first that yields a usable timetable wins.
// In practice that is the agency's own address followed by the catalogue's
// mirror: the mirrors lag, sometimes by months — BART's was serving a timetable
// that had expired six days earlier, Reading's one that ran out in June — so a
// mirror is a fallback for when an agency is unreachable, never the source.
//
// Returns false when nothing needed doing, which is the common case.
func (s *Store) Refresh(id string, urls ...string) (bool, error) {
	if id == "" || len(urls) == 0 {
		return false, fmt.Errorf("a feed needs an id and a url")
	}
	var errs []string
	for _, u := range urls {
		if strings.TrimSpace(u) == "" {
			continue
		}
		changed, err := s.refreshFrom(id, u)
		if err == nil {
			return changed, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", u, err))
	}
	return false, fmt.Errorf("every source failed — %s", strings.Join(errs, "; "))
}

func (s *Store) refreshFrom(id, url string) (bool, error) {

	var etag, modified string
	if existing, ok := s.Get(id); ok {
		etag, modified = existing.Meta.ETag, existing.Meta.Modified
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "mu/1.0 (+https://github.com/micro/mu)")
	// Ask before taking. An unchanged feed answers 304 and costs nothing.
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("could not reach the feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("feed returned %d", resp.StatusCode)
	}

	// Download to a file rather than memory: these are tens of megabytes, and
	// zip needs to seek anyway.
	tmp, err := os.CreateTemp(s.dir, "."+id+".*.zip")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		tmp.Close()
		return false, fmt.Errorf("download failed: %w", err)
	}
	if n == maxFeedBytes {
		tmp.Close()
		return false, fmt.Errorf("feed is larger than %dMB, which is beyond what this holds", maxFeedBytes>>20)
	}
	if n == 0 {
		tmp.Close()
		return false, fmt.Errorf("feed was empty")
	}

	buildDir := filepath.Join(s.dir, id+".tmp")
	os.RemoveAll(buildDir)
	meta, err := Build(id, tmp, n, buildDir, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"))
	if err != nil {
		tmp.Close()
		os.RemoveAll(buildDir)
		return false, fmt.Errorf("could not build the feed: %w", err)
	}
	tmp.Close()

	// An expired timetable is worse than no new timetable: it answers every
	// question with silence. Refuse it so the next source gets a turn, and so
	// whatever is already loaded stays loaded.
	if expired, end := meta.Expired(time.Now()); expired {
		os.RemoveAll(buildDir)
		return false, fmt.Errorf("timetable ran out on %s", end.Format("2 Jan 2006"))
	}

	// Prove the built index answers before anything is thrown away.
	fresh, err := Open(buildDir)
	if err != nil {
		os.RemoveAll(buildDir)
		return false, fmt.Errorf("built an index that would not open: %w", err)
	}

	final := filepath.Join(s.dir, id)
	old := filepath.Join(s.dir, "."+id+".old")
	os.RemoveAll(old)

	// From here on the window where nothing is in place is two renames wide,
	// and the previous copy still exists on disk until the new one is serving.
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			fresh.Close()
			os.RemoveAll(buildDir)
			return false, fmt.Errorf("could not move the old feed aside: %w", err)
		}
	}
	if err := os.Rename(buildDir, final); err != nil {
		// Put it back rather than leaving the instance with nothing.
		os.Rename(old, final)
		fresh.Close()
		os.RemoveAll(buildDir)
		return false, fmt.Errorf("could not move the new feed into place: %w", err)
	}
	fresh.Close()

	// Reopen from the final path so the handle does not point at a directory
	// name that no longer exists.
	live, err := Open(final)
	if err != nil {
		os.Rename(final, buildDir)
		if os.Rename(old, final) == nil {
			if back, err2 := Open(final); err2 == nil {
				s.swap(id, back)
			}
		}
		return false, fmt.Errorf("could not open the installed feed: %w", err)
	}

	s.swap(id, live)
	os.RemoveAll(old)
	return true, nil
}

// swap replaces a live index and closes the one it displaced.
func (s *Store) swap(id string, idx *Index) {
	s.mu.Lock()
	prev := s.feeds[id]
	s.feeds[id] = idx
	s.mu.Unlock()

	// The displaced index may still be mid-answer, so it is closed a little
	// later rather than under a reader's feet. A held file descriptor for a
	// deleted file costs nothing on unix.
	if prev != nil {
		time.AfterFunc(time.Minute, func() { prev.Close() })
	}
}

// Age reports how long ago a feed was built.
func (s *Store) Age(id string) (time.Duration, bool) {
	idx, ok := s.Get(id)
	if !ok || idx.Meta.BuiltAt == 0 {
		return 0, false
	}
	return time.Since(time.Unix(idx.Meta.BuiltAt, 0)), true
}
