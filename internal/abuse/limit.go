// Package abuse bounds work before it starts. Counters are committed before
// admission, so failed work and process restarts cannot replenish an allowance.
package abuse

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"mu/internal/dir"
	"mu/internal/settings"
)

var state struct {
	sync.Mutex
	once    sync.Once
	db      *sql.DB
	err     error
	cleanup int64
}

func open() {
	path := filepath.Join(dir.Data(), "abuse.db")
	if state.err = os.MkdirAll(filepath.Dir(path), 0700); state.err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		state.err = err
		return
	}
	f.Close()
	state.db, state.err = sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(2000)")
	if state.err != nil {
		return
	}
	state.db.SetMaxOpenConns(1)
	_, state.err = state.db.Exec(`CREATE TABLE IF NOT EXISTS limits (key TEXT PRIMARY KEY, count INTEGER NOT NULL, expires INTEGER NOT NULL)`)
}

// Limit reads a positive operator override. Invalid values retain protection.
func Limit(name string, fallback int) int {
	n, err := strconv.Atoi(settings.Get(name))
	if err == nil && n > 0 {
		return n
	}
	return fallback
}

// Take consumes an attempt. A positive wait means refused. Keys are hashed so
// the store contains neither raw IPs nor account names. Expired rows are pruned,
// and a full table refuses new identities instead of growing without bound.
func Take(key string, limit int, window time.Duration) (time.Duration, error) {
	state.Lock()
	defer state.Unlock()
	state.once.Do(open)
	if state.err != nil {
		return 0, fmt.Errorf("abuse protection unavailable: %w", state.err)
	}
	now := time.Now().Unix()
	if now >= state.cleanup {
		if _, err := state.db.Exec(`DELETE FROM limits WHERE expires <= ?`, now); err != nil {
			return 0, err
		}
		state.cleanup = now + 60
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
	var count int
	var expires int64
	err := state.db.QueryRow(`SELECT count, expires FROM limits WHERE key = ?`, hash).Scan(&count, &expires)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if err == sql.ErrNoRows {
		var rows int
		if err := state.db.QueryRow(`SELECT COUNT(*) FROM limits`).Scan(&rows); err != nil {
			return 0, err
		}
		if rows >= 20000 {
			return time.Minute, nil
		}
	}
	if expires <= now {
		count = 0
		expires = now + int64(window/time.Second)
	}
	if count >= limit {
		return time.Duration(expires-now) * time.Second, nil
	}
	_, err = state.db.Exec(`INSERT INTO limits(key,count,expires) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET count=excluded.count, expires=excluded.expires`, hash, count+1, expires)
	return 0, err
}

var active = struct {
	sync.Mutex
	counts map[string]int
}{counts: map[string]int{}}

// Start bounds account work, including failed attempts, independently of credit
// billing. Active slots disappear on restart because the work also disappears;
// the hourly and daily attempt budgets remain on disk.
func Start(account, kind string, hourly, daily, concurrent int) (func(), error) {
	key := kind + ":" + account
	active.Lock()
	if active.counts[key] >= concurrent {
		active.Unlock()
		return nil, fmt.Errorf("too many %s requests running; try again shortly", kind)
	}
	active.counts[key]++
	active.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			active.Lock()
			defer active.Unlock()
			active.counts[key]--
			if active.counts[key] == 0 {
				delete(active.counts, key)
			}
		})
	}
	for _, b := range []struct {
		suffix string
		limit  int
		window time.Duration
	}{{":hour", hourly, time.Hour}, {":day", daily, 24 * time.Hour}} {
		wait, err := Take(key+b.suffix, b.limit, b.window)
		if err != nil {
			release()
			return nil, err
		}
		if wait > 0 {
			release()
			return nil, fmt.Errorf("%s request limit reached; retry in %d seconds", kind, int(wait.Seconds()))
		}
	}
	return release, nil
}
