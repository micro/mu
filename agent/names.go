package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mu/internal/data"
	"mu/internal/userdb"
)

// Reservations outlive deletion, so an old link cannot be claimed by another
// account. Display names and owner-scoped mail tags can change independently.
const namesFile = "agents/names.json"

var namesMu sync.Mutex

// Published maps are immutable. Mutations save a copy before replacing them.
var namesCache map[string]nameOwner
var namesByAgent map[nameOwner]string

type nameOwner struct {
	Owner string `json:"owner"`
	ID    string `json:"id"`
}

// names migrates the complete roster (not a paginated account view). Existing
// reservations always win; otherwise the oldest record wins, with ID breaking
// ties. Save the whole index atomically before returning any assigned address.
func names() (map[string]nameOwner, error) {
	namesMu.Lock()
	defer namesMu.Unlock()
	if namesCache != nil {
		return namesCache, nil
	}
	reserved := map[string]nameOwner{}
	raw, err := data.LoadFile(namesFile)
	if err == nil {
		if err = json.Unmarshal(raw, &reserved); err != nil {
			return nil, err
		}
		if reserved == nil {
			return nil, fmt.Errorf("invalid agent name reservations")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	records, err := userdb.Snapshot(ns, collection)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Created.Equal(records[j].Created) {
			return records[i].ID < records[j].ID
		}
		return records[i].Created.Before(records[j].Created)
	})
	assigned := map[nameOwner]bool{}
	for _, ref := range reserved {
		assigned[ref] = true
	}
	// Protect existing candidate names before assigning suffixes: a duplicate
	// "research" must not take another existing agent's "research2".
	candidates := map[string]bool{}
	for _, rec := range records {
		if a := fromRecord(rec.Owner, rec); a != nil {
			candidates[nameBase(a)] = true
		}
	}
	changed := false
	for _, rec := range records {
		a := fromRecord(rec.Owner, rec)
		if a == nil {
			continue
		}
		ref := nameOwner{a.Owner, a.ID}
		if assigned[ref] {
			continue
		}
		base := nameBase(a)
		candidate := base
		for n := 2; reservedName(candidate) || reserved[candidate].ID != "" || (candidate != base && candidates[candidate]); n++ {
			candidate = base + strconv.Itoa(n)
		}
		reserved[candidate] = ref
		assigned[ref] = true
		changed = true
	}
	if changed {
		if err := data.SaveJSON(namesFile, reserved); err != nil {
			return nil, err
		}
	}
	publishNames(reserved)
	return reserved, nil
}

func reservedName(name string) bool {
	switch name {
	case "micro", "code", "new", "connect", "agents", "run", "pending", "session", "flow", "handoff", "title":
		return true
	}
	return Platform(name) != nil
}

func namedAgent(name string) (*Agent, error) {
	reserved, err := names()
	if err != nil {
		return nil, err
	}
	ref, ok := reserved[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, nil
	}
	return For(ref.Owner, ref.ID), nil
}

// SetProfile controls only the public name/description page, never execution or
// the userdb record's public flag (which would disclose the agent's prompt).
func SetProfile(owner, id string, public bool) error {
	rosterMutation.Lock()
	defer rosterMutation.Unlock()
	a := For(owner, id)
	if a == nil {
		return fmt.Errorf("no such agent")
	}
	a.ProfilePublic = public
	return a.save()
}

func nameBase(a *Agent) string {
	base := slugify(a.Tag)
	if base == "" {
		base = slugify(a.Name)
	}
	if base == "" {
		base = "agent"
	}
	return base
}

func publishNames(reserved map[string]nameOwner) {
	reverse := make(map[nameOwner]string, len(reserved))
	for name, ref := range reserved {
		reverse[ref] = name
	}
	namesCache, namesByAgent = reserved, reverse
}

func reservedSlug(a *Agent) (string, error) {
	if _, err := names(); err != nil {
		return "", err
	}
	namesMu.Lock()
	defer namesMu.Unlock()
	return namesByAgent[nameOwner{a.Owner, a.ID}], nil
}

// reserveName adds only the newly created agent; ordinary lookups never reload
// the roster. The old immutable index stays valid if persistence fails.
func reserveName(a *Agent) error {
	if _, err := names(); err != nil {
		return err
	}
	namesMu.Lock()
	defer namesMu.Unlock()
	ref := nameOwner{a.Owner, a.ID}
	if namesByAgent[ref] != "" {
		return nil
	}
	next := make(map[string]nameOwner, len(namesCache)+1)
	for name, owner := range namesCache {
		next[name] = owner
	}
	base := nameBase(a)
	candidate := base
	for n := 2; reservedName(candidate) || next[candidate].ID != ""; n++ {
		candidate = base + strconv.Itoa(n)
	}
	next[candidate] = ref
	if err := data.SaveJSON(namesFile, next); err != nil {
		return err
	}
	publishNames(next)
	return nil
}
