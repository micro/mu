package agent

import (
	"sort"
	"sync"
	"time"

	"mu/internal/data"

	"github.com/google/uuid"
)

// Flow represents a saved agent query with tool calls and rendered response.
type Flow struct {
	ID        string     `json:"id"`
	AccountID string     `json:"account_id"`
	Prompt    string     `json:"prompt"`
	Steps     []FlowStep `json:"steps"`
	Answer    string     `json:"answer"`    // markdown answer text
	HTML      string     `json:"html"`      // rendered HTML (set on completion)
	Status    string     `json:"status"`    // "running", "done", "error"
	Error     string     `json:"error"`     // error message if status is "error"
	ParentID  string     `json:"parent_id"` // prior flow ID for multi-turn chains
	CreatedAt time.Time  `json:"created_at"`
}

// FlowStep records one tool call and its result within a flow.
type FlowStep struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Result string         `json:"result"`
}

var (
	flowMu    sync.RWMutex
	flowStore = map[string]*Flow{} // id → flow
)

func init() {
	var flows []*Flow
	if err := data.LoadJSON("agent_flows.json", &flows); err == nil {
		for _, f := range flows {
			// Backfill status for pre-existing flows
			if f.Status == "" && f.Answer != "" {
				f.Status = "done"
			}
			flowStore[f.ID] = f
		}
	}
}

// maxFlowsPerUser is the maximum number of flows kept per user.
// When exceeded, the oldest completed flows are evicted.
const maxFlowsPerUser = 200

// saveFlow persists a new flow or updates an existing one.
func saveFlow(f *Flow) error {
	flowMu.Lock()
	defer flowMu.Unlock()
	flowStore[f.ID] = f
	evictOldFlows(f.AccountID)
	return persistFlows()
}

// evictOldFlows removes the oldest completed flows for an account when
// the per-user limit is exceeded. Caller must hold flowMu.
func evictOldFlows(accountID string) {
	var userFlows []*Flow
	for _, f := range flowStore {
		if f.AccountID == accountID {
			userFlows = append(userFlows, f)
		}
	}
	if len(userFlows) <= maxFlowsPerUser {
		return
	}
	// Sort oldest first
	sort.Slice(userFlows, func(i, j int) bool {
		return userFlows[i].CreatedAt.Before(userFlows[j].CreatedAt)
	})
	// Delete oldest completed flows until within limit
	toRemove := len(userFlows) - maxFlowsPerUser
	for _, f := range userFlows {
		if toRemove <= 0 {
			break
		}
		if f.Status == "done" || f.Status == "error" {
			delete(flowStore, f.ID)
			toRemove--
		}
	}
}

// getFlow returns the flow with the given ID, or nil if not found.
func getFlow(id string) *Flow {
	flowMu.RLock()
	defer flowMu.RUnlock()
	return flowStore[id]
}

// updateFlow applies a mutation to a flow in-place and persists.
func updateFlow(id string, fn func(f *Flow)) {
	flowMu.Lock()
	defer flowMu.Unlock()
	f, ok := flowStore[id]
	if !ok {
		return
	}
	fn(f)
	persistFlows() //nolint:errcheck
}

// getLatestRunningFlow returns the most recent "running" flow for an account, or nil.
func getLatestRunningFlow(accountID string) *Flow {
	flowMu.RLock()
	defer flowMu.RUnlock()
	var latest *Flow
	for _, f := range flowStore {
		if f.AccountID == accountID && f.Status == "running" {
			if latest == nil || f.CreatedAt.After(latest.CreatedAt) {
				latest = f
			}
		}
	}
	return latest
}

// ListFlows returns all flows belonging to accountID, newest first.
func ListFlows(accountID string) []*Flow {
	flowMu.RLock()
	defer flowMu.RUnlock()
	var out []*Flow
	for _, f := range flowStore {
		if f.AccountID == accountID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// deleteFlow removes a flow owned by accountID.
func deleteFlow(accountID, id string) error {
	flowMu.Lock()
	defer flowMu.Unlock()
	f, ok := flowStore[id]
	if !ok || f.AccountID != accountID {
		return nil
	}
	delete(flowStore, id)
	return persistFlows()
}

// newFlowID returns a new unique flow ID.
func newFlowID() string {
	return uuid.New().String()
}

// getConversationHistory walks the parent chain from a flow and returns
// up to maxTurns prior turns in chronological order (oldest first).
func getConversationHistory(flowID string, maxTurns int) []*Flow {
	var chain []*Flow
	seen := map[string]bool{}
	id := flowID
	for i := 0; i < maxTurns && id != ""; i++ {
		if seen[id] {
			break
		}
		seen[id] = true
		f := getFlow(id)
		if f == nil {
			break
		}
		chain = append(chain, f)
		id = f.ParentID
	}
	// Reverse to chronological order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// persistFlows writes the in-memory store to disk. Caller must hold flowMu.
func persistFlows() error {
	flows := make([]*Flow, 0, len(flowStore))
	for _, f := range flowStore {
		flows = append(flows, f)
	}
	return data.SaveJSON("agent_flows.json", flows)
}
