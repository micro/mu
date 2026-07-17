// Package userdb is the shared collections data layer behind mu.db. A record has
// a server-set owner and a private/public flag; reads are owner-scoped and writes
// are owner-checked. A namespace isolates one logical store — the app SDK uses
// "apps/{slug}" so each app's data is separate, while the MCP/REST surface uses
// "api" so a caller's records live alongside everyone's under owner scoping.
//
// Security: the owner is always the authenticated caller, set by the calling
// layer from the session — never taken from untrusted input. There is no query
// language (records are JSON filtered in Go), so no injection surface, and keys
// are confined by internal/data.
package userdb

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/data"

	"github.com/google/uuid"
)

const (
	// MaxRecords caps records per owner per collection.
	MaxRecords = 2000
	// MaxRecordSize caps a single record's JSON size.
	MaxRecordSize = 64 * 1024
	// MaxListLimit caps how many records a single list returns.
	MaxListLimit = 200
)

// Errors returned by the store.
var (
	ErrBadCollection = errors.New("invalid collection name")
	ErrBadNamespace  = errors.New("invalid namespace")
	ErrNoData        = errors.New("data is required")
	ErrTooLarge      = errors.New("record exceeds 64KB limit")
	ErrFull          = errors.New("you have too many records in this collection")
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("not your record")
	ErrAuth          = errors.New("authentication required")
)

// safeSegment keeps namespace/collection names to a path-free charset.
var safeSegment = regexp.MustCompile(`^[a-z0-9][a-z0-9_/-]{0,79}$`)
var collectionRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

var mu sync.Mutex

// Record is one stored item. Owner and timestamps are managed here; Data is the
// caller's arbitrary JSON payload.
type Record struct {
	ID      string                 `json:"id"`
	Owner   string                 `json:"owner"`
	Public  bool                   `json:"public"`
	Data    map[string]interface{} `json:"data"`
	Created time.Time              `json:"created"`
	Updated time.Time              `json:"updated"`
}

func key(ns, collection string) (string, error) {
	if !safeSegment.MatchString(ns) || strings.Contains(ns, "..") {
		return "", ErrBadNamespace
	}
	if !collectionRe.MatchString(collection) {
		return "", ErrBadCollection
	}
	return ns + "/db/" + collection + ".json", nil
}

// Create stores a new record owned by owner. owner must be non-empty.
func Create(ns, owner, collection string, dataObj map[string]interface{}, public bool) (*Record, error) {
	if owner == "" {
		return nil, ErrAuth
	}
	k, err := key(ns, collection)
	if err != nil {
		return nil, err
	}
	if len(dataObj) == 0 {
		return nil, ErrNoData
	}
	if b, _ := json.Marshal(dataObj); len(b) > MaxRecordSize {
		return nil, ErrTooLarge
	}
	mu.Lock()
	defer mu.Unlock()
	recs := load(k)
	owned := 0
	for i := range recs {
		if recs[i].Owner == owner {
			owned++
		}
	}
	if owned >= MaxRecords {
		return nil, ErrFull
	}
	now := time.Now()
	rec := Record{ID: uuid.New().String(), Owner: owner, Public: public, Data: dataObj, Created: now, Updated: now}
	recs = append(recs, rec)
	data.SaveJSON(k, recs)
	return &rec, nil
}

// Get returns a record the caller may see (public, or owned). caller "" is a
// guest and may only see public records.
func Get(ns, caller, collection, id string) (*Record, error) {
	k, err := key(ns, collection)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	recs := load(k)
	mu.Unlock()
	for i := range recs {
		if recs[i].ID == id {
			if recs[i].Public || (caller != "" && recs[i].Owner == caller) {
				r := recs[i]
				return &r, nil
			}
			break
		}
	}
	return nil, ErrNotFound
}

// List returns records for scope "mine" (default), "public" or "all", with an
// optional equality/operator filter, sort field and limit. A guest (caller "")
// is forced to "public".
func List(ns, caller, collection, scope string, where map[string]interface{}, sortField, order string, limit int) ([]Record, error) {
	k, err := key(ns, collection)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	recs := load(k)
	mu.Unlock()
	if scope == "" {
		scope = "mine"
	}
	if caller == "" {
		scope = "public"
	}
	out := filter(recs, scope, caller, where)
	sortRecords(out, sortField, order)
	if limit <= 0 || limit > MaxListLimit {
		limit = MaxListLimit
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Update replaces a record's data (and public flag). Owner only.
func Update(ns, caller, collection, id string, dataObj map[string]interface{}, public bool) (*Record, error) {
	if caller == "" {
		return nil, ErrAuth
	}
	k, err := key(ns, collection)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	recs := load(k)
	for i := range recs {
		if recs[i].ID != id {
			continue
		}
		if recs[i].Owner != caller {
			return nil, ErrForbidden
		}
		if len(dataObj) > 0 {
			if b, _ := json.Marshal(dataObj); len(b) > MaxRecordSize {
				return nil, ErrTooLarge
			}
			recs[i].Data = dataObj
		}
		recs[i].Public = public
		recs[i].Updated = time.Now()
		data.SaveJSON(k, recs)
		r := recs[i]
		return &r, nil
	}
	return nil, ErrNotFound
}

// Delete removes a record. Owner only.
func Delete(ns, caller, collection, id string) error {
	if caller == "" {
		return ErrAuth
	}
	k, err := key(ns, collection)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	recs := load(k)
	for i := range recs {
		if recs[i].ID == id {
			if recs[i].Owner != caller {
				return ErrForbidden
			}
			recs = append(recs[:i], recs[i+1:]...)
			data.SaveJSON(k, recs)
			return nil
		}
	}
	return ErrNotFound
}

func load(k string) []Record {
	b, err := data.LoadFile(k)
	if err != nil {
		return nil
	}
	var recs []Record
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil
	}
	return recs
}

func filter(recs []Record, scope, caller string, where map[string]interface{}) []Record {
	out := make([]Record, 0, len(recs))
	for _, rec := range recs {
		switch scope {
		case "public":
			if !rec.Public {
				continue
			}
		case "all":
			if !rec.Public && rec.Owner != caller {
				continue
			}
		default: // "mine"
			if rec.Owner != caller {
				continue
			}
		}
		if !matchesWhere(rec, where) {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// matchesWhere reports whether a record satisfies every field condition. A
// condition is either a scalar (equality) or an operator object, e.g.
// {"age":{"gte":18},"title":{"contains":"note"}}.
func matchesWhere(rec Record, where map[string]interface{}) bool {
	for field, cond := range where {
		got, present := rec.Data[field]
		if !matchField(got, present, cond) {
			return false
		}
	}
	return true
}

var whereOps = map[string]bool{
	"eq": true, "ne": true, "gt": true, "gte": true, "lt": true,
	"lte": true, "contains": true, "in": true, "exists": true,
}

func opMap(cond interface{}) (map[string]interface{}, bool) {
	m, ok := cond.(map[string]interface{})
	if !ok || len(m) == 0 {
		return nil, false
	}
	for k := range m {
		if !whereOps[k] {
			return nil, false
		}
	}
	return m, true
}

func matchField(got interface{}, present bool, cond interface{}) bool {
	ops, isOps := opMap(cond)
	if !isOps {
		return present && jsonEqual(got, cond)
	}
	for op, val := range ops {
		if !applyOp(got, present, op, val) {
			return false
		}
	}
	return true
}

func applyOp(got interface{}, present bool, op string, val interface{}) bool {
	switch op {
	case "exists":
		want, _ := val.(bool)
		return present == want
	case "eq":
		return present && jsonEqual(got, val)
	case "ne":
		return !present || !jsonEqual(got, val)
	case "gt":
		return present && cmpValue(got, val) > 0
	case "gte":
		return present && cmpValue(got, val) >= 0
	case "lt":
		return present && cmpValue(got, val) < 0
	case "lte":
		return present && cmpValue(got, val) <= 0
	case "contains":
		if !present {
			return false
		}
		if arr, ok := got.([]interface{}); ok {
			for _, e := range arr {
				if jsonEqual(e, val) {
					return true
				}
			}
			return false
		}
		return strings.Contains(strings.ToLower(toStr(got)), strings.ToLower(toStr(val)))
	case "in":
		if !present {
			return false
		}
		arr, ok := val.([]interface{})
		if !ok {
			return false
		}
		for _, e := range arr {
			if jsonEqual(got, e) {
				return true
			}
		}
		return false
	}
	return false
}

func cmpValue(a, b interface{}) int {
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	return strings.Compare(toStr(a), toStr(b))
}

func jsonEqual(a, b interface{}) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func sortRecords(recs []Record, field, order string) {
	desc := order != "asc"
	sort.SliceStable(recs, func(i, j int) bool {
		var less bool
		if field == "" {
			less = recs[i].Updated.Before(recs[j].Updated)
		} else {
			less = cmpValue(recs[i].Data[field], recs[j].Data[field]) < 0
		}
		if desc {
			return !less
		}
		return less
	})
}

func toStr(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
