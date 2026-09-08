package maps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/blob"
)

// World tiles stay on this origin. The upstream request identifies the project,
// never the instance or the viewer. Cache for seven days as OSM's tile policy
// requires when upstream expiry headers are not interpreted.
func worldTile(w http.ResponseWriter, r *http.Request, parts []string) {
	z, e1 := strconv.Atoi(parts[0])
	x, e2 := strconv.Atoi(parts[1])
	y, e3 := strconv.Atoi(strings.TrimSuffix(parts[2], ".png"))
	if e1 != nil || e2 != nil || e3 != nil || z < 1 || z > 19 || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		http.NotFound(w, r)
		return
	}
	key := fmt.Sprintf("tiles/world/%d/%d/%d.json", z, x, y)
	var cached struct {
		At   time.Time
		Data []byte
	}
	if b, err := blob.Get(key); err == nil {
		_ = json.Unmarshal(b, &cached)
	}
	if len(cached.Data) == 0 || time.Since(cached.At) >= 7*24*time.Hour {
		_, acc := auth.TrySession(r)
		if acc == nil {
			http.Error(w, "Sign in to load new map tiles", http.StatusUnauthorized)
			return
		}
		if err := mayFetch(acc.ID); err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		u := fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", z, x, y)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
		if err != nil {
			http.Error(w, "Could not request map tile", http.StatusBadGateway)
			return
		}
		req.Header.Set("User-Agent", "Mu/1.0 (+https://github.com/micro/mu)")
		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Could not load map tile", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "Map tile unavailable", http.StatusBadGateway)
			return
		}
		b, err := readAll(resp)
		if err != nil {
			http.Error(w, "Could not read map tile", http.StatusBadGateway)
			return
		}
		cached.At, cached.Data = time.Now(), b
		stored, _ := json.Marshal(cached)
		if err := blob.Put(key, stored, "application/json"); err != nil {
			http.Error(w, "Could not cache map tile", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(cached.Data)
}
