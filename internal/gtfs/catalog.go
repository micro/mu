package gtfs

// Finding out who publishes a timetable for somewhere.
//
// There is no global transit API, but there is a global list of the agencies
// that publish one: the Mobility Database catalogue, a CSV of some 3,400 feeds
// with their download addresses and the box of the world each covers. It is
// keyless, which is the only reason any of this fits a repository that wants to
// be cloned and run.
//
// Roughly a third of the static feeds want a key of some kind; those are
// skipped, because a feed you have to sign up for is exactly the barrier this
// is here to remove.

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// catalogURL is the published CSV of every known feed.
var catalogURL = "https://storage.googleapis.com/storage/v1/b/mdb-csv/o/sources.csv?alt=media"

// Feed is one agency's published timetable.
type Feed struct {
	ID       string
	Provider string
	Country  string
	Place    string
	// Direct is the agency's own address and Mirror is the catalogue's copy.
	// Order matters: mirrors lag, sometimes by months.
	Direct string
	Mirror string
	// The box the feed covers, when the catalogue knows it.
	MinLat, MaxLat float64
	MinLon, MaxLon float64
	HasBox         bool
}

// covers reports whether a point falls inside the feed's box.
func (f Feed) covers(lat, lon float64) bool {
	return f.HasBox && lat >= f.MinLat && lat <= f.MaxLat && lon >= f.MinLon && lon <= f.MaxLon
}

// area is the size of the box, used to prefer the most specific feed.
//
// A point in Reading sits inside both Reading Buses and any national feed that
// covers Great Britain. The smaller box is the one that knows the street.
func (f Feed) area() float64 {
	if !f.HasBox {
		return 1e18
	}
	return (f.MaxLat - f.MinLat) * (f.MaxLon - f.MinLon)
}

var (
	catalogOnce sync.Once
	catalogMu   sync.RWMutex
	catalog     []Feed
	catalogAt   time.Time
)

// Catalog returns the known feeds, fetching the list at most once a week.
//
// The list changes when an agency joins or moves, which is not often, and it is
// several hundred kilobytes — so this is cached hard and never fetched on the
// path of a caller's question.
func Catalog() ([]Feed, error) {
	catalogMu.RLock()
	fresh := time.Since(catalogAt) < 7*24*time.Hour && len(catalog) > 0
	cached := catalog
	catalogMu.RUnlock()
	if fresh {
		return cached, nil
	}

	feeds, err := fetchCatalog()
	if err != nil {
		if len(cached) > 0 {
			return cached, nil // stale beats nothing
		}
		return nil, err
	}
	catalogMu.Lock()
	catalog, catalogAt = feeds, time.Now()
	catalogMu.Unlock()
	return feeds, nil
}

func fetchCatalog() ([]Feed, error) {
	req, err := http.NewRequest(http.MethodGet, catalogURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mu/1.0 (+https://github.com/micro/mu)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the feed catalogue is unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the feed catalogue returned %d", resp.StatusCode)
	}

	cr := csv.NewReader(io.LimitReader(resp.Body, 32<<20))
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}

	var out []Feed
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		get := func(k string) string {
			i, ok := col[k]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		if get("data_type") != "gtfs" {
			continue // realtime feeds are a different shape and not read here
		}
		// A feed behind a key is a barrier, which is the thing we are removing.
		if get("urls.authentication_type") != "" {
			continue
		}
		f := Feed{
			ID:       feedSlug(get("mdb_source_id"), get("provider")),
			Provider: get("provider"),
			Country:  get("location.country_code"),
			Place: strings.TrimSpace(strings.Join([]string{
				get("location.municipality"), get("location.subdivision_name"),
			}, " ")),
			Direct: get("urls.direct_download"),
			Mirror: get("urls.latest"),
		}
		if f.Direct == "" && f.Mirror == "" {
			continue
		}
		minLat, e1 := strconv.ParseFloat(get("location.bounding_box.minimum_latitude"), 64)
		maxLat, e2 := strconv.ParseFloat(get("location.bounding_box.maximum_latitude"), 64)
		minLon, e3 := strconv.ParseFloat(get("location.bounding_box.minimum_longitude"), 64)
		maxLon, e4 := strconv.ParseFloat(get("location.bounding_box.maximum_longitude"), 64)
		if e1 == nil && e2 == nil && e3 == nil && e4 == nil {
			f.MinLat, f.MaxLat, f.MinLon, f.MaxLon = minLat, maxLat, minLon, maxLon
			f.HasBox = true
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the feed catalogue was empty")
	}
	return out, nil
}

// feedSlug makes a filesystem-safe id.
func feedSlug(id, provider string) string {
	p := strings.ToLower(provider)
	var b strings.Builder
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "feed"
	}
	if id == "" {
		return slug
	}
	return slug + "-" + id
}

// FeedsFor returns the feeds covering a point, most specific first.
func FeedsFor(lat, lon float64, limit int) ([]Feed, error) {
	all, err := Catalog()
	if err != nil {
		return nil, err
	}
	var hits []Feed
	for _, f := range all {
		if f.covers(lat, lon) {
			hits = append(hits, f)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].area() < hits[j].area() })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// FindFeed looks a feed up by id, provider or place.
//
// Ranked rather than first-past-the-post, which is not fussiness: over a
// catalogue of this size the first partial match is usually the wrong one.
// "metrobus" found Metrobus Transit of St. John's, Newfoundland, ahead of the
// Metrobus that runs buses around Gatwick, because Newfoundland came first in
// the file. An exact name has to beat a name that merely contains it.
func FindFeed(q string) (Feed, bool) {
	all, err := Catalog()
	if err != nil {
		return Feed{}, false
	}
	q = strings.TrimSpace(q)
	lower := strings.ToLower(q)
	if lower == "" {
		return Feed{}, false
	}

	// Lower is better. Nothing beyond a place match is considered a match.
	const (
		rankID = iota
		rankProvider
		rankProviderPrefix
		rankProviderContains
		rankPlace
		rankNone
	)

	best, bestRank := -1, rankNone
	for i := range all {
		p := strings.ToLower(all[i].Provider)
		var rank int
		switch {
		case strings.EqualFold(all[i].ID, q):
			return all[i], true
		case p == lower:
			rank = rankProvider
		case strings.HasPrefix(p, lower):
			rank = rankProviderPrefix
		case strings.Contains(p, lower):
			rank = rankProviderContains
		case strings.Contains(strings.ToLower(all[i].Place), lower):
			rank = rankPlace
		default:
			continue
		}
		if rank < bestRank {
			best, bestRank = i, rank
		}
	}
	if best < 0 {
		return Feed{}, false
	}
	return all[best], true
}
