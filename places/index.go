package places

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/asim/quadtree"
	_ "modernc.org/sqlite"

	"mu/app"
	"mu/data"
)

// schemaVersion is the current places database schema version.
// Bumping this constant causes all place data to be wiped on the next startup,
// discarding rows produced by incompatible previous data sources (e.g. Overpass,
// Foursquare) so that fresh data from the current source can be indexed cleanly.
const schemaVersion = "v2"

var (
	placesDB    *sql.DB
	placesDBMu  sync.Mutex
	placesDBOne sync.Once
)

// geohash base32 alphabet
const ghChars = "0123456789bcdefghjkmnpqrstuvwxyz"

// encodeGeohash encodes lat/lon into a geohash string of the given precision.
func encodeGeohash(lat, lon float64, precision int) string {
	minLat, maxLat := -90.0, 90.0
	minLon, maxLon := -180.0, 180.0
	result := make([]byte, precision)
	bits := 0
	hashVal := 0
	isEven := true

	for i := 0; i < precision; {
		if isEven {
			mid := (minLon + maxLon) / 2
			if lon >= mid {
				hashVal = (hashVal << 1) | 1
				minLon = mid
			} else {
				hashVal <<= 1
				maxLon = mid
			}
		} else {
			mid := (minLat + maxLat) / 2
			if lat >= mid {
				hashVal = (hashVal << 1) | 1
				minLat = mid
			} else {
				hashVal <<= 1
				maxLat = mid
			}
		}
		isEven = !isEven
		bits++
		if bits == 5 {
			result[i] = ghChars[hashVal]
			i++
			bits = 0
			hashVal = 0
		}
	}
	return string(result)
}

// initPlacesDB opens (or creates) the dedicated places SQLite database.
func initPlacesDB() error {
	var initErr error
	placesDBOne.Do(func() {
		dir := os.ExpandEnv("$HOME/.mu")
		dbPath := filepath.Join(dir, "data", "places.db")
		os.MkdirAll(filepath.Dir(dbPath), 0700)

		var err error
		placesDB, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=10000")
		if err != nil {
			initErr = fmt.Errorf("places db open: %w", err)
			return
		}
		placesDB.SetMaxOpenConns(4)
		placesDB.SetMaxIdleConns(4)

		// Check the stored schema version.  If it is absent (unversioned data
		// from a previous Overpass/Foursquare-backed schema) or does not match
		// schemaVersion, wipe all place data so stale rows no longer block
		// queries to Google Places.
		var storedVer string
		_ = placesDB.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&storedVer)
		if storedVer != schemaVersion {
			app.Log("places", "places db version mismatch (have %q, want %q) – wiping data", storedVer, schemaVersion)
			if _, err = placesDB.Exec(`DROP TABLE IF EXISTS places_fts`); err != nil {
				initErr = fmt.Errorf("places db wipe fts: %w", err)
				return
			}
			if _, err = placesDB.Exec(`DROP TABLE IF EXISTS places`); err != nil {
				initErr = fmt.Errorf("places db wipe places: %w", err)
				return
			}
			if _, err = placesDB.Exec(`DROP TABLE IF EXISTS schema_version`); err != nil {
				initErr = fmt.Errorf("places db wipe version: %w", err)
				return
			}
		}

		_, err = placesDB.Exec(`
			CREATE TABLE IF NOT EXISTS schema_version (
				version TEXT NOT NULL
			);
			CREATE TABLE IF NOT EXISTS places (
				id           TEXT PRIMARY KEY,
				name         TEXT NOT NULL,
				category     TEXT,
				address      TEXT,
				lat          REAL NOT NULL,
				lon          REAL NOT NULL,
				geohash      TEXT,
				phone        TEXT,
				website      TEXT,
				opening_hours TEXT,
				cuisine      TEXT,
				indexed_at   DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_places_lat     ON places(lat);
			CREATE INDEX IF NOT EXISTS idx_places_lon     ON places(lon);
			CREATE INDEX IF NOT EXISTS idx_places_geohash ON places(geohash);

			CREATE VIRTUAL TABLE IF NOT EXISTS places_fts USING fts5(
				id       UNINDEXED,
				name,
				category,
				address,
				cuisine,
				tokenize='unicode61 remove_diacritics 1'
			);
		`)
		if err != nil {
			initErr = fmt.Errorf("places db schema: %w", err)
			return
		}

		// Persist the version record when the DB is freshly created or wiped.
		if storedVer != schemaVersion {
			if _, err = placesDB.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
				initErr = fmt.Errorf("places db version insert: %w", err)
				return
			}
		}
	})
	return initErr
}

// getPlacesDB returns the shared places database, initialising it if needed.
func getPlacesDB() (*sql.DB, error) {
	if err := initPlacesDB(); err != nil {
		return nil, err
	}
	return placesDB, nil
}

// indexPlaces batch-upserts places into the SQLite places table and FTS index.
func indexPlaces(places []*Place) {
	if len(places) == 0 {
		return
	}
	db, err := getPlacesDB()
	if err != nil {
		app.Log("places", "indexPlaces: DB error: %v", err)
		return
	}

	placesDBMu.Lock()
	defer placesDBMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		app.Log("places", "indexPlaces: begin tx: %v", err)
		return
	}

	mainStmt, err := tx.Prepare(`
		INSERT INTO places (id, name, category, address, lat, lon, geohash, phone, website, opening_hours, cuisine, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, category=excluded.category, address=excluded.address,
			lat=excluded.lat, lon=excluded.lon, geohash=excluded.geohash,
			phone=excluded.phone, website=excluded.website,
			opening_hours=excluded.opening_hours, cuisine=excluded.cuisine,
			indexed_at=excluded.indexed_at
	`)
	if err != nil {
		tx.Rollback()
		app.Log("places", "indexPlaces: prepare: %v", err)
		return
	}
	defer mainStmt.Close()

	ftsDelStmt, err := tx.Prepare(`DELETE FROM places_fts WHERE id = ?`)
	if err != nil {
		tx.Rollback()
		app.Log("places", "indexPlaces: prepare fts del: %v", err)
		return
	}
	defer ftsDelStmt.Close()

	ftsInsStmt, err := tx.Prepare(`INSERT INTO places_fts (id, name, category, address, cuisine) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		app.Log("places", "indexPlaces: prepare fts ins: %v", err)
		return
	}
	defer ftsInsStmt.Close()

	now := time.Now()
	for _, p := range places {
		gh := encodeGeohash(p.Lat, p.Lon, 6)
		if _, err := mainStmt.Exec(p.ID, p.Name, p.Category, p.Address,
			p.Lat, p.Lon, gh, p.Phone, p.Website, p.OpeningHours, p.Cuisine, now); err != nil {
			app.Log("places", "indexPlaces: insert %s: %v", p.ID, err)
			continue
		}
		ftsDelStmt.Exec(p.ID)
		if _, err := ftsInsStmt.Exec(p.ID, p.Name, p.Category, p.Address, p.Cuisine); err != nil {
			app.Log("places", "indexPlaces: fts insert %s: %v", p.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		app.Log("places", "indexPlaces: commit: %v", err)
	}
}

// sanitizeFTSQuery converts a raw query into a safe FTS5 MATCH expression.
// Each word is treated as a quoted literal prefix match.
func sanitizeFTSQuery(q string) string {
	q = strings.Map(func(r rune) rune {
		switch r {
		case '"', '\'', '(', ')', '*', '+', '^', '-', '~', ':', '.':
			return ' '
		}
		return r
	}, q)
	words := strings.Fields(q)
	if len(words) == 0 {
		return ""
	}
	for i, w := range words {
		words[i] = `"` + strings.ToLower(w) + `"*`
	}
	return strings.Join(words, " ")
}

// searchPlacesFTS searches the local SQLite index using FTS5 and an optional
// bounding-box geo filter.  Results are sorted by distance when hasRef is true.
func searchPlacesFTS(query string, refLat, refLon float64, radiusM int, hasRef bool) ([]*Place, error) {
	db, err := getPlacesDB()
	if err != nil {
		return nil, err
	}

	const limit = 500
	var rows *sql.Rows

	switch {
	case query != "" && hasRef:
		latDelta := float64(radiusM) / 111000.0
		lonDelta := float64(radiusM) / (111000.0 * math.Cos(refLat*math.Pi/180))
		ftsQ := sanitizeFTSQuery(query)
		if ftsQ == "" {
			return nil, nil
		}
		rows, err = db.Query(`
			SELECT p.id, p.name, p.category, p.address, p.lat, p.lon,
			       p.phone, p.website, p.opening_hours, p.cuisine
			FROM places p
			WHERE p.lat BETWEEN ? AND ?
			  AND p.lon BETWEEN ? AND ?
			  AND p.id IN (SELECT id FROM places_fts WHERE places_fts MATCH ?)
			LIMIT ?`,
			refLat-latDelta, refLat+latDelta,
			refLon-lonDelta, refLon+lonDelta,
			ftsQ, limit)

	case query != "":
		ftsQ := sanitizeFTSQuery(query)
		if ftsQ == "" {
			return nil, nil
		}
		rows, err = db.Query(`
			SELECT p.id, p.name, p.category, p.address, p.lat, p.lon,
			       p.phone, p.website, p.opening_hours, p.cuisine
			FROM places p
			WHERE p.id IN (SELECT id FROM places_fts WHERE places_fts MATCH ?)
			LIMIT ?`,
			ftsQ, limit)

	case hasRef:
		latDelta := float64(radiusM) / 111000.0
		lonDelta := float64(radiusM) / (111000.0 * math.Cos(refLat*math.Pi/180))
		rows, err = db.Query(`
			SELECT id, name, category, address, lat, lon,
			       phone, website, opening_hours, cuisine
			FROM places
			WHERE lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?
			LIMIT ?`,
			refLat-latDelta, refLat+latDelta,
			refLon-lonDelta, refLon+lonDelta,
			limit)

	default:
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("places FTS query: %w", err)
	}
	defer rows.Close()

	var result []*Place
	for rows.Next() {
		p := &Place{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Address,
			&p.Lat, &p.Lon, &p.Phone, &p.Website, &p.OpeningHours, &p.Cuisine); err != nil {
			continue
		}
		if hasRef {
			dist := haversine(refLat, refLon, p.Lat, p.Lon)
			if dist > float64(radiusM) {
				continue // outside actual radius (bounding box is an approximation)
			}
			p.Distance = dist
		}
		result = append(result, p)
	}

	if hasRef {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Distance < result[j].Distance
		})
	}
	return result, nil
}

// startHourlyRefresh launches a background goroutine that cycles through the
// known cities once per hour, refreshing each city's place index from Overpass.
// Used only when no Google API key is configured.
func startHourlyRefresh() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		cityIdx := 0
		for range ticker.C {
			if len(cities) == 0 {
				continue
			}
			city := cities[cityIdx%len(cities)]
			cityIdx++
			app.Log("places", "Hourly refresh: fetching places for %s", city.Name)
			radiusM := int(city.RadiusKm * 1000)
			places, err := fetchCityFromOverpass(city.Lat, city.Lon, radiusM)
			if err != nil {
				app.Log("places", "Hourly refresh failed for %s: %v", city.Name, err)
				continue
			}
			if err := data.SaveJSON(cacheFileKey(city.Name), places); err != nil {
				app.Log("places", "Hourly refresh: save failed for %s: %v", city.Name, err)
			}
			mutex.Lock()
			for _, p := range places {
				qtree.Insert(quadtree.NewPoint(p.Lat, p.Lon, p))
			}
			mutex.Unlock()
			go indexPlaces(places)
			app.Log("places", "Hourly refresh: indexed %d places for %s", len(places), city.Name)
		}
	}()
}

