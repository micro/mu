package agent

import (
	"sort"
	"sync"
	"time"

	"mu/data"

	"github.com/google/uuid"
)

// Flow represents a saved agent query with tool calls and rendered response.
type Flow struct {
	ID        string     `json:"id"`
	AccountID string     `json:"account_id"`
	Prompt    string     `json:"prompt"`
	Steps     []FlowStep `json:"steps"`
	Answer    string     `json:"answer"` // markdown answer text
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
			flowStore[f.ID] = f
		}
	}
}

// saveFlow persists a new flow or updates an existing one.
func saveFlow(f *Flow) error {
	flowMu.Lock()
	defer flowMu.Unlock()
	flowStore[f.ID] = f
	return persistFlows()
}

// getFlow returns the flow with the given ID, or nil if not found.
func getFlow(id string) *Flow {
	flowMu.RLock()
	defer flowMu.RUnlock()
	return flowStore[id]
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

// persistFlows writes the in-memory store to disk. Caller must hold flowMu.
func persistFlows() error {
	flows := make([]*Flow, 0, len(flowStore))
	for _, f := range flowStore {
		flows = append(flows, f)
	}
	return data.SaveJSON("agent_flows.json", flows)
}
