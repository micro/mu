package agent

import (
	"sort"
	"sync"
	"time"

	"mu/internal/data"
	"mu/internal/thread"

	"github.com/google/uuid"
)

// Flow represents a saved agent query with tool calls and rendered response.
type Flow struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Prompt    string `json:"prompt"`
	// Title is a name given to the conversation this flow starts, replacing the
	// first prompt in the rail. Only ever set on a root flow.
	Title  string     `json:"title,omitempty"`
	Steps  []FlowStep `json:"steps"`
	Answer string     `json:"answer"` // markdown answer text
	HTML   string     `json:"html"`   // rendered HTML (set on completion)
	Status string     `json:"status"` // "running", "done", "error"
	Error  string     `json:"error"`  // error message if status is "error"
	Agent  string     `json:"agent"`  // user-defined agent id used for this turn ("" = default)
	// Source is what triggered the run — "" for the page, "mail", "schedule".
	// Empty on every flow written before runs could start without somebody
	// watching, which is why the page treats empty as the page.
	Source string `json:"source,omitempty"`
	// Trigger names who set it off, for the ones nobody asked for in person:
	// "email from asim@aslam.me".
	Trigger  string `json:"trigger,omitempty"`
	ParentID string `json:"parent_id"` // prior flow ID for multi-turn chains
	// Via is which client the turn arrived through and which conversation on
	// it, and is what lets the next message find this one. See thread.go.
	Via       Via       `json:"via,omitzero"`
	CreatedAt time.Time `json:"created_at"`
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

// ForgetConversation drops every run that made up a deleted conversation.
//
// Wired to thread.Deleted. Without it, deleting a conversation removed the
// record of it and left the runs, and adoptAll — which exists to carry chains
// written before the record across into it — read those runs at the next
// start-up and put the conversation back. It could not tell "never adopted"
// from "adopted, then deleted by the person who owned it", because absence was
// the only thing it had to go on.
//
// Keyed the way adopt keys it: a web conversation's thread Key is the id of the
// chain's first run, which is what the web has always keyed a conversation on.
// Anything from another client — mail, SMS, a room — has no chain here and
// nothing to drop.
func ForgetConversation(t thread.Thread) {
	if t.Client != thread.WebClient || t.Key == "" {
		return
	}
	for _, f := range sessionChain(t.Account, t.Key) {
		deleteFlow(t.Account, f.ID) //nolint:errcheck
	}
}

// newFlowID returns a new unique flow ID.
func newFlowID() string {
	return uuid.New().String()
}

// session is one chat conversation as the web used to keep them: a chain of
// turns linked by ParentID. RootID (the first turn) is its stable identity.
//
// Not what a conversation is any more — that is internal/thread. This survives
// for one reason, adoption: the chains written before the record existed are
// somebody's history and have to be read once to be carried across.
type session struct {
	RootID    string // stable id of the conversation's first turn
	HeadID    string // latest turn in the chain
	Title     string // the conversation's first prompt
	Agent     string // which agent it was with ("" = the default)
	UpdatedAt time.Time
	Turns     int
}

// flowSessions groups an account's flows into conversations (ParentID chains),
// newest first. A conversation's head is its latest turn — a flow that is not
// any other flow's parent.
//
// Runs that arrived some other way are not chat conversations, so they are not
// adopted into one: an email is a conversation on mail and the record already
// has it as that.
func flowSessions(accountID string) []session {
	var flows []*Flow
	for _, f := range ListFlows(accountID) { // newest first
		if startedHere(f.Source) {
			flows = append(flows, f)
		}
	}
	byID := make(map[string]*Flow, len(flows))
	isParent := make(map[string]bool, len(flows))
	for _, f := range flows {
		byID[f.ID] = f
		if f.ParentID != "" {
			isParent[f.ParentID] = true
		}
	}
	var sessions []session
	for _, f := range flows {
		if isParent[f.ID] {
			continue // not a leaf — a later turn continues it
		}
		title := f.Prompt
		rootID := f.ID
		turns := 0
		seen := map[string]bool{}
		for id := f.ID; id != "" && !seen[id]; {
			seen[id] = true
			cur := byID[id]
			if cur == nil {
				break
			}
			turns++
			title = cur.Prompt // ends at the root's prompt
			if cur.Title != "" {
				title = cur.Title // a name somebody gave it wins
			}
			rootID = cur.ID // ends at the root's id
			id = cur.ParentID
		}
		sessions = append(sessions, session{RootID: rootID, HeadID: f.ID, Title: title,
			Agent: f.Agent, UpdatedAt: f.CreatedAt, Turns: turns})
	}
	return sessions
}

// startedHere says whether a run came from the chat on this page. Empty is the
// page: every flow written before a run could start anywhere else has no source,
// and there was nowhere else for it to have come from.
func startedHere(source string) bool {
	return source == "" || source == thread.WebClient
}

// pastTurns is a conversation's prior turns, oldest first, as pairs.
//
// The shape the hand-rolled pipeline wants, filled from the record rather than
// from the workflow chain it used to walk. A run is evicted at 200 per account
// and a message is not, so reading history out of runs quietly truncated a long
// conversation and there was no way to tell from the page.
func pastTurns(accountID, threadID string, turns int) []*Flow {
	msgs := thread.Messages(accountID, threadID, turns*2)
	var out []*Flow
	for _, m := range msgs {
		if m.Role == thread.RoleAgent {
			if len(out) > 0 && out[len(out)-1].Answer == "" {
				out[len(out)-1].Answer = m.Text
			}
			continue
		}
		out = append(out, &Flow{Prompt: m.Text})
	}
	// A question nobody answered is not a turn: it is the message being answered
	// now, already written down before the run starts.
	for len(out) > 0 && out[len(out)-1].Answer == "" {
		out = out[:len(out)-1]
	}
	return out
}

// adopt brings a conversation that predates the record into it.
//
// The web kept conversations as chains of workflow records for a year before
// there was a record, and those chains are somebody's history: a rail that
// stopped listing them would read as the product having deleted them. So the
// first time one is opened — or once, at startup, for the rail — it is written
// into the record as what it always was. Idempotent: keyed on the chain's root,
// which is what the web has always keyed its conversation on.
func adopt(accountID string, chain []*Flow) string {
	if len(chain) == 0 {
		return ""
	}
	root := chain[0]
	if th := thread.Find(accountID, thread.WebClient, root.ID); th != nil {
		return th.ID
	}
	th := thread.Open(accountID, thread.WebClient, root.ID)
	if th == nil {
		return ""
	}
	thread.SetAgent(accountID, th.ID, chain[len(chain)-1].Agent)
	for _, f := range chain {
		thread.Add(thread.Message{Thread: th.ID, Account: accountID,
			Role: thread.RolePerson, Text: f.Prompt, At: f.CreatedAt})
		thread.Add(thread.Message{Thread: th.ID, Account: accountID,
			Role: thread.RoleAgent, Text: f.Answer, At: f.CreatedAt, Workflow: f.ID})
	}
	return th.ID
}

// adoptAll adopts every conversation the chat still holds only as workflow
// records. Runs once, at start-up, so the rail has them the first time it is
// drawn rather than only after somebody opens one.
func adoptAll() {
	accounts := map[string]bool{}
	flowMu.RLock()
	for _, f := range flowStore {
		if f.AccountID != "" && startedHere(f.Source) {
			accounts[f.AccountID] = true
		}
	}
	flowMu.RUnlock()

	for id := range accounts {
		for _, s := range flowSessions(id) {
			if thread.Find(id, thread.WebClient, s.RootID) == nil {
				adopt(id, sessionChain(id, s.RootID))
			}
		}
	}
}

// sessionChain returns a conversation's turns (oldest first) given ANY flow id
// in the chain. It walks up to the root, then forward to the newest leaf, so a
// stale mid-chain id (e.g. an older head in a bookmarked URL) still resolves the
// whole conversation and its current head.
func sessionChain(accountID, anyID string) []*Flow {
	// Walk up to the root.
	seen := map[string]bool{}
	root := getFlow(anyID)
	for root != nil && root.ParentID != "" && !seen[root.ID] {
		seen[root.ID] = true
		p := getFlow(root.ParentID)
		if p == nil {
			break
		}
		root = p
	}
	if root == nil {
		return nil
	}
	// Index children so we can walk forward to the newest leaf.
	flows := ListFlows(accountID) // newest first
	childrenOf := map[string][]*Flow{}
	for _, f := range flows {
		if f.ParentID != "" {
			childrenOf[f.ParentID] = append(childrenOf[f.ParentID], f)
		}
	}
	var chain []*Flow
	visited := map[string]bool{}
	for cur := root; cur != nil && !visited[cur.ID]; {
		visited[cur.ID] = true
		chain = append(chain, cur)
		kids := childrenOf[cur.ID]
		if len(kids) == 0 {
			break
		}
		next := kids[0] // newest child (list is newest-first)
		for _, k := range kids {
			if k.CreatedAt.After(next.CreatedAt) {
				next = k
			}
		}
		cur = next
	}
	return chain
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
