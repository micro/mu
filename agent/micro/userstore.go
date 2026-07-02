package micro

// User-defined agents: an account can create its own micro-agents (name +
// system prompt + tool set) that run through the same executor as the built-ins.
// Stored per account and resolved by the executor via UserAgentResolver.

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"sync"

	"mu/internal/data"
)

var (
	uaMu       sync.RWMutex
	userAgents = map[string]map[string]*Agent{} // accountID → id → agent
	uaFile     = "user_agents.json"
	uaOnce     sync.Once
)

func init() { UserAgentResolver = GetUserAgentFor }

func loadUserAgents() {
	uaOnce.Do(func() {
		uaMu.Lock()
		defer uaMu.Unlock()
		data.LoadJSON(uaFile, &userAgents)
	})
}

// NewUserAgentID returns a fresh user-agent id ("u_" + 12 hex chars).
func NewUserAgentID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "u_" + hex.EncodeToString(b)
}

// SaveUserAgent creates or updates an account's agent. The agent's ID and
// OwnerAccountID are set/enforced here.
func SaveUserAgent(accountID string, a *Agent) *Agent {
	loadUserAgents()
	uaMu.Lock()
	defer uaMu.Unlock()
	if a.ID == "" || !strings.HasPrefix(a.ID, "u_") {
		a.ID = NewUserAgentID()
	}
	a.OwnerAccountID = accountID
	if userAgents[accountID] == nil {
		userAgents[accountID] = map[string]*Agent{}
	}
	userAgents[accountID][a.ID] = a
	data.SaveJSON(uaFile, userAgents)
	return a
}

// GetUserAgentFor returns an account's agent by ID, or nil.
func GetUserAgentFor(accountID, id string) *Agent {
	loadUserAgents()
	uaMu.RLock()
	defer uaMu.RUnlock()
	if m := userAgents[accountID]; m != nil {
		return m[id]
	}
	return nil
}

// UserAgentsFor returns an account's agents, sorted by name.
func UserAgentsFor(accountID string) []*Agent {
	loadUserAgents()
	uaMu.RLock()
	defer uaMu.RUnlock()
	var out []*Agent
	for _, a := range userAgents[accountID] {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DeleteUserAgentFor removes an account's agent.
func DeleteUserAgentFor(accountID, id string) {
	loadUserAgents()
	uaMu.Lock()
	defer uaMu.Unlock()
	if m := userAgents[accountID]; m != nil {
		delete(m, id)
		data.SaveJSON(uaFile, userAgents)
	}
}

// DeleteUserAgents removes all of an account's agents (account teardown).
func DeleteUserAgents(accountID string) {
	loadUserAgents()
	uaMu.Lock()
	defer uaMu.Unlock()
	if _, ok := userAgents[accountID]; ok {
		delete(userAgents, accountID)
		data.SaveJSON(uaFile, userAgents)
	}
}
