package apps

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"

	"github.com/google/uuid"
)

// The app data layer: collections of JSON records with a private/per-user/public
// model. Unlike the flat key-value store (mu.store), records have an owner and a
// visibility flag, so one universal app can hold each user's private data plus a
// shared public set — e.g. a notes app where "my notes" are private and "public
// notes" are readable by everyone.
//
// Security: the owner is always the authenticated session account, set
// server-side and never taken from the client (the same rule as mu.store and the
// account binding across the agent tools). Reads enforce visibility; writes
// require ownership.

const (
	// MaxDBRecords caps records per app+collection.
	MaxDBRecords = 2000
	// MaxDBListLimit caps how many records a single list returns.
	MaxDBListLimit = 200
)

// collectionRe keeps collection names to a safe, path-free charset.
var collectionRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// dbMu guards read-modify-write on the per-collection JSON files.
var dbMu sync.Mutex

// Record is one stored item. Owner and timestamps are server-managed; Data is
// the app's arbitrary JSON payload.
type Record struct {
	ID      string                 `json:"id"`
	Owner   string                 `json:"owner"`
	Public  bool                   `json:"public"`
	Data    map[string]interface{} `json:"data"`
	Created time.Time              `json:"created"`
	Updated time.Time              `json:"updated"`
}

// handleSDKDB serves the collections data API at /apps/{slug}/sdk/db.
//
// Body: {op, collection, id?, data?, public?, scope?, where?, sort?, order?, limit?}
//   - create: owner = session account, returns the record
//   - get:    returns a record the caller may see (public, or owned)
//   - list:   scope "mine" (default) | "public" | "all", optional where/sort/limit
//   - update: owner only; replaces data, may toggle public
//   - delete: owner only
//
// Reads work for guests but only see public records; every write needs a session.
func handleSDKDB(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc := auth.TrySession(r)

	var req struct {
		Op         string                 `json:"op"`
		Collection string                 `json:"collection"`
		ID         string                 `json:"id"`
		Data       map[string]interface{} `json:"data"`
		Public     bool                   `json:"public"`
		Scope      string                 `json:"scope"`
		Where      map[string]interface{} `json:"where"`
		Sort       string                 `json:"sort"`
		Order      string                 `json:"order"`
		Limit      int                    `json:"limit"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if !collectionRe.MatchString(req.Collection) {
		app.RespondError(w, http.StatusBadRequest, "Invalid collection name")
		return
	}

	key := "apps/" + slug + "/db/" + req.Collection + ".json"

	// Reads may be done by guests (public only); writes require a session.
	write := req.Op == "create" || req.Op == "update" || req.Op == "delete"
	if write && acc == nil {
		app.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	caller := ""
	if acc != nil {
		caller = acc.ID
	}

	switch req.Op {
	case "create":
		if len(req.Data) == 0 {
			app.RespondError(w, http.StatusBadRequest, "data is required")
			return
		}
		if b, _ := json.Marshal(req.Data); len(b) > MaxStoreValueSize {
			app.RespondError(w, http.StatusBadRequest, "Record exceeds 64KB limit")
			return
		}
		dbMu.Lock()
		defer dbMu.Unlock()
		recs := loadRecords(key)
		if len(recs) >= MaxDBRecords {
			app.RespondError(w, http.StatusBadRequest, "Collection is full")
			return
		}
		now := time.Now()
		rec := Record{
			ID:      uuid.New().String(),
			Owner:   caller,
			Public:  req.Public,
			Data:    req.Data,
			Created: now,
			Updated: now,
		}
		recs = append(recs, rec)
		data.SaveJSON(key, recs)
		app.RespondJSON(w, map[string]interface{}{"record": rec})

	case "get":
		dbMu.Lock()
		recs := loadRecords(key)
		dbMu.Unlock()
		for _, rec := range recs {
			if rec.ID == req.ID {
				if rec.Public || (caller != "" && rec.Owner == caller) {
					app.RespondJSON(w, map[string]interface{}{"record": rec})
					return
				}
				break
			}
		}
		app.RespondError(w, http.StatusNotFound, "Not found")

	case "list":
		dbMu.Lock()
		recs := loadRecords(key)
		dbMu.Unlock()
		scope := req.Scope
		if scope == "" {
			scope = "mine"
		}
		// Guests can only ever see public records.
		if caller == "" {
			scope = "public"
		}
		out := filterRecords(recs, scope, caller, req.Where)
		sortRecords(out, req.Sort, req.Order)
		limit := req.Limit
		if limit <= 0 || limit > MaxDBListLimit {
			limit = MaxDBListLimit
		}
		if len(out) > limit {
			out = out[:limit]
		}
		app.RespondJSON(w, map[string]interface{}{"records": out})

	case "update":
		dbMu.Lock()
		defer dbMu.Unlock()
		recs := loadRecords(key)
		for i := range recs {
			if recs[i].ID != req.ID {
				continue
			}
			if recs[i].Owner != caller {
				app.RespondError(w, http.StatusForbidden, "Not your record")
				return
			}
			if len(req.Data) > 0 {
				if b, _ := json.Marshal(req.Data); len(b) > MaxStoreValueSize {
					app.RespondError(w, http.StatusBadRequest, "Record exceeds 64KB limit")
					return
				}
				recs[i].Data = req.Data
			}
			recs[i].Public = req.Public
			recs[i].Updated = time.Now()
			data.SaveJSON(key, recs)
			app.RespondJSON(w, map[string]interface{}{"record": recs[i]})
			return
		}
		app.RespondError(w, http.StatusNotFound, "Not found")

	case "delete":
		dbMu.Lock()
		defer dbMu.Unlock()
		recs := loadRecords(key)
		for i := range recs {
			if recs[i].ID == req.ID {
				if recs[i].Owner != caller {
					app.RespondError(w, http.StatusForbidden, "Not your record")
					return
				}
				recs = append(recs[:i], recs[i+1:]...)
				data.SaveJSON(key, recs)
				app.RespondJSON(w, map[string]string{"status": "ok"})
				return
			}
		}
		app.RespondError(w, http.StatusNotFound, "Not found")

	default:
		app.RespondError(w, http.StatusBadRequest, "Invalid operation. Use create, get, list, update or delete")
	}
}

// filterRecords selects records by scope (mine/public/all) for caller and an
// optional equality filter on data fields.
func filterRecords(recs []Record, scope, caller string, where map[string]interface{}) []Record {
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
// {"age":{"gte":18},"title":{"contains":"note"},"done":{"ne":true}}.
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

// opMap returns cond as an operator object when every key is a known operator;
// otherwise the condition is a plain equality match.
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

// cmpValue orders two decoded-JSON values: numbers numerically, else by string.
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
	as, bs := toStr(a), toStr(b)
	return strings.Compare(as, bs)
}

// jsonEqual compares two decoded-JSON values for equality.
func jsonEqual(a, b interface{}) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// sortRecords orders records by a data field (or Updated when field is empty),
// descending by default.
func sortRecords(recs []Record, field, order string) {
	desc := order != "asc"
	sort.SliceStable(recs, func(i, j int) bool {
		var less bool
		if field == "" {
			less = recs[i].Updated.Before(recs[j].Updated)
		} else {
			less = lessValue(recs[i].Data[field], recs[j].Data[field])
		}
		if desc {
			return !less
		}
		return less
	})
}

// lessValue orders two decoded-JSON values: numbers numerically, everything else
// by string form.
func lessValue(a, b interface{}) bool {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return af < bf
	}
	return toStr(a) < toStr(b)
}

func toStr(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// loadRecords reads a collection file. Must be called under dbMu for writes.
func loadRecords(key string) []Record {
	b, err := data.LoadFile(key)
	if err != nil {
		return nil
	}
	var recs []Record
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil
	}
	return recs
}
