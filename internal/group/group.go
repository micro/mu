// Package group owns shared membership so services can check access without
// depending on each other. Invitations do not grant access until accepted.
package group

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

const Changed = "groups.changed"
const storeKey = "groups.json"
const RoomPrefix = "group_"

type Invitation struct {
	Account   string    `json:"account"`
	InvitedBy string    `json:"invited_by"`
	Expires   time.Time `json:"expires"`
}
type Group struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Encrypted   bool                  `json:"encrypted"`
	Owner       string                `json:"owner"`
	Created     time.Time             `json:"created"`
	Members     map[string]string     `json:"members"`
	Invitations map[string]Invitation `json:"invitations,omitempty"`
}
type Pending struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Invitation
}

var mu sync.RWMutex
var records = map[string]Group{}
var loaded bool
var loadErr error

// Load fails closed on damaged storage; it must never overwrite an unreadable store.
func Load() error {
	mu.Lock()
	defer mu.Unlock()
	if loaded {
		return loadErr
	}
	loaded = true
	b, err := data.LoadFile(storeKey)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		loadErr = err
		return err
	}
	var next map[string]Group
	if err = json.Unmarshal(b, &next); err != nil || next == nil {
		loadErr = fmt.Errorf("cannot read group membership")
		return loadErr
	}
	for id, g := range next {
		if g.ID != id || g.Owner == "" || g.Members[g.Owner] != "owner" {
			loadErr = fmt.Errorf("invalid group membership")
			return loadErr
		}
	}
	records = next
	return nil
}

func clone(g Group) Group {
	m := map[string]string{}
	for a, role := range g.Members {
		m[a] = role
	}
	g.Members = m
	invites := map[string]Invitation{}
	for a, v := range g.Invitations {
		invites[a] = v
	}
	g.Invitations = invites
	return g
}
func save(next map[string]Group) error {
	if err := data.SaveJSON(storeKey, next); err != nil {
		return err
	}
	records = next
	return nil
}
func copyRecords() map[string]Group {
	next := make(map[string]Group, len(records))
	for id, g := range records {
		next[id] = g
	}
	return next
}
func Member(id, account string) bool { return Role(id, account) != "" }
func Role(id, account string) string {
	if account == "" || Load() != nil {
		return ""
	}
	mu.RLock()
	defer mu.RUnlock()
	return records[id].Members[account]
}
func Read(id, account string) (Group, error) {
	if err := Load(); err != nil {
		return Group{}, err
	}
	mu.RLock()
	defer mu.RUnlock()
	g, ok := records[id]
	if !ok || account == "" || g.Members[account] == "" {
		return Group{}, errors.New("group not found")
	}
	out := clone(g)
	if out.Members[account] == "member" {
		out.Invitations = nil
	}
	return out, nil
}
func List(account string) ([]Group, []Pending, error) {
	if account == "" {
		return nil, nil, errors.New("sign in to view groups")
	}
	if err := Load(); err != nil {
		return nil, nil, err
	}
	mu.RLock()
	defer mu.RUnlock()
	groups := []Group{}
	pending := []Pending{}
	for _, g := range records {
		if g.Members[account] != "" {
			c := clone(g)
			if c.Members[account] == "member" {
				c.Invitations = nil
			}
			groups = append(groups, c)
		} else if inv, ok := g.Invitations[account]; ok && time.Now().Before(inv.Expires) {
			pending = append(pending, Pending{ID: g.ID, Name: g.Name, Invitation: inv})
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Created.After(groups[j].Created) })
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	return groups, pending, nil
}
func Create(account, name string, encrypted bool) (Group, error) {
	name = strings.TrimSpace(name)
	if account == "" || name == "" || len([]rune(name)) > 100 {
		return Group{}, errors.New("a group needs a name of up to 100 characters")
	}
	if err := Load(); err != nil {
		return Group{}, err
	}
	mu.Lock()
	defer mu.Unlock()
	count := 0
	for _, g := range records {
		if g.Owner == account {
			count++
		}
	}
	if count >= 50 {
		return Group{}, errors.New("you can own up to 50 groups")
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Group{}, err
	}
	g := Group{ID: hex.EncodeToString(b[:]), Name: name, Encrypted: encrypted, Owner: account, Created: time.Now().UTC(), Members: map[string]string{account: "owner"}, Invitations: map[string]Invitation{}}
	next := copyRecords()
	next[g.ID] = g
	if err := save(next); err != nil {
		return Group{}, err
	}
	return clone(g), nil
}

// Change applies a single membership operation and commits it before it becomes
// visible to access checks. Owners cannot accidentally leave an ownerless group.
func Change(id, actor, action, target, value string) error {
	if actor == "" {
		return errors.New("sign in to manage groups")
	}
	if err := Load(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	stored, ok := records[id]
	if !ok {
		return errors.New("group not found")
	}
	g := clone(stored)
	role := g.Members[actor]
	if action == "accept" || action == "decline" {
		inv, ok := g.Invitations[actor]
		if !ok || !time.Now().Before(inv.Expires) {
			return errors.New("invitation not found or expired")
		}
		delete(g.Invitations, actor)
		if action == "accept" {
			g.Members[actor] = "member"
		}
	} else {
		if role == "" {
			return errors.New("group not found")
		}
		admin := role == "owner" || role == "admin"
		switch action {
		case "invite":
			if !admin {
				return errors.New("only owners and admins can invite people")
			}
			if target == "" || g.Members[target] != "" {
				return errors.New("choose someone who is not already a member")
			}
			for a, inv := range g.Invitations {
				if time.Now().After(inv.Expires) {
					delete(g.Invitations, a)
				}
			}
			if len(g.Members)+len(g.Invitations) >= 100 {
				return errors.New("a group can have up to 100 members and invitations")
			}
			g.Invitations[target] = Invitation{Account: target, InvitedBy: actor, Expires: time.Now().UTC().Add(7 * 24 * time.Hour)}
		case "remove":
			if target == g.Owner {
				return errors.New("transfer ownership before leaving")
			}
			if target != actor && (!admin || (role != "owner" && g.Members[target] == "admin")) {
				return errors.New("you cannot remove this member")
			}
			delete(g.Members, target)
			delete(g.Invitations, target)
		case "role":
			if role != "owner" || g.Members[target] == "" || target == actor {
				return errors.New("only the owner can change another member's role")
			}
			if value != "member" && value != "admin" && value != "owner" {
				return errors.New("invalid role")
			}
			if value == "owner" {
				g.Members[actor] = "admin"
				g.Owner = target
			}
			g.Members[target] = value
		case "rename":
			value = strings.TrimSpace(value)
			if !admin || value == "" || len([]rune(value)) > 100 {
				return errors.New("an owner or admin must supply a name of up to 100 characters")
			}
			g.Name = value
		case "delete":
			if role != "owner" {
				return errors.New("only the owner can delete the group")
			}
			next := copyRecords()
			delete(next, id)
			return save(next)
		default:
			return errors.New("unknown group action")
		}
	}
	next := copyRecords()
	next[id] = g
	return save(next)
}

// Forget removes a deleted identity, including pending invitations. Owned groups
// are closed rather than silently granting ownership to somebody else.
func Forget(account string) error {
	if err := Load(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	next := copyRecords()
	for id, stored := range next {
		if stored.Owner == account {
			delete(next, id)
			continue
		}
		g := clone(stored)
		delete(g.Members, account)
		delete(g.Invitations, account)
		for a, inv := range g.Invitations {
			if inv.InvitedBy == account {
				delete(g.Invitations, a)
			}
		}
		next[id] = g
	}
	return save(next)
}

// Details is for internal resource owners after their entry-point access check.
// It returns a copy, never a mutable reference to the membership store.
func Details(id string) (Group, bool) {
	if Load() != nil {
		return Group{}, false
	}
	mu.RLock()
	defer mu.RUnlock()
	g, ok := records[id]
	return clone(g), ok
}
