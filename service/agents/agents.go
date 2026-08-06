// Package agents is where a person creates and scopes the agents that act for
// them.
//
// Mu is tools for agents, and until now there was no agent — there was one chat
// box at /agent and a token page where you could mint a credential with your
// whole account behind it. Anything you handed that token to *was* you: it could
// read your mail because you can, spend your credits because you can, and
// nothing recorded that you had given it away or what you meant it to do.
//
// An agent is the smaller thing you actually meant. It has a name, so it can be
// talked about. It has a scope, so it reaches some services and not others. It
// has one token, so it can be revoked on its own without taking your other
// agents with it. And it is either hosted here or external — the difference
// between something this instance runs and something you run elsewhere that
// holds a credential to call in.
//
// The scope is the point. Everything else is bookkeeping around it.
package agents

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/userdb"
)

const (
	ns         = "agents"
	collection = "agents"
)

// Kind is where an agent runs.
const (
	// Hosted runs on this instance: you give it instructions and it executes
	// here, against its own scope.
	Hosted = "hosted"
	// External runs wherever you run it — Claude Desktop, Cursor, your own
	// program — and calls in with its token.
	External = "external"
)

// Agent is one named, scoped actor belonging to an account.
type Agent struct {
	ID    string `json:"id"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	// Services is what this agent may reach. Empty means everything the owner
	// can reach, which is the old behaviour of a bare token and is deliberately
	// possible to choose — but it is a choice, not a default.
	Services []string `json:"services,omitempty"`
	// Prompt is the standing instruction for a hosted agent: what it is for.
	Prompt string `json:"prompt,omitempty"`
	// TokenID identifies the credential without holding it. The secret is shown
	// once at creation and never stored in readable form.
	TokenID  string    `json:"token_id,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitempty"`
}

// Unscoped reports whether this agent can reach everything its owner can.
func (a *Agent) Unscoped() bool { return len(a.Services) == 0 }

// Create makes an agent and issues its token, returning the secret exactly
// once.
//
// The token is created with the agent's scope baked into its permissions, so
// the confinement travels with the credential rather than being re-derived at
// each call site. A caller that never reads this package still cannot escape
// the scope, because the check lives at the MCP boundary.
func Create(owner, name, kind, prompt string, services []string) (*Agent, string, error) {
	if owner == "" {
		return nil, "", fmt.Errorf("sign in to create an agent")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("an agent needs a name")
	}
	if len(name) > 60 {
		return nil, "", fmt.Errorf("that name is too long")
	}
	if kind != Hosted && kind != External {
		kind = External
	}

	services = validServices(services)

	a := &Agent{
		Owner:    owner,
		Name:     name,
		Kind:     kind,
		Services: services,
		Prompt:   strings.TrimSpace(prompt),
		Created:  time.Now(),
	}

	// One token per agent, named for it, so the token page and any audit shows
	// which agent a credential belongs to rather than a row called "token 3".
	tok, secret, err := auth.CreateToken(owner, "agent: "+name, auth.ScopeFor(services), time.Time{})
	if err != nil {
		return nil, "", fmt.Errorf("could not issue a token for this agent: %w", err)
	}
	a.TokenID = tok.ID

	rec, err := userdb.Create(ns, owner, collection, fields(a), false)
	if err != nil {
		// Do not leave a live credential behind an agent that was not stored.
		_ = auth.DeleteToken(tok.ID, owner)
		return nil, "", err
	}
	a.ID = rec.ID
	return a, secret, nil
}

// validServices keeps only names this instance actually registers, so a scope
// cannot name something that does not exist and quietly allow nothing.
func validServices(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		if _, known := service.SpecFor(s); !known {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func fields(a *Agent) map[string]any {
	return map[string]any{
		"name": a.Name, "kind": a.Kind, "prompt": a.Prompt,
		"token_id": a.TokenID, "services": strings.Join(a.Services, ","),
		"created": a.Created.Format(time.RFC3339),
	}
}

// List returns an owner's agents, newest first.
func List(owner string) []*Agent {
	if owner == "" {
		return nil
	}
	records, err := userdb.List(ns, owner, collection, "own", nil, "", "", 200)
	if err != nil {
		return nil
	}
	var out []*Agent
	for i := range records {
		if a := fromRecord(owner, records[i]); a != nil {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// Get returns one agent the owner owns, or nil.
func Get(owner, id string) *Agent {
	for _, a := range List(owner) {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// Remove deletes an agent and revokes its token.
//
// Both, always. An agent removed from the list while its credential kept
// working would be the worst of both: gone from the page that would have told
// you it existed, and still able to call.
func Remove(owner, id string) error {
	a := Get(owner, id)
	if a == nil {
		return fmt.Errorf("no such agent")
	}
	if a.TokenID != "" {
		_ = auth.DeleteToken(a.TokenID, owner)
	}
	return userdb.Delete(ns, owner, collection, id)
}

func fromRecord(owner string, rec userdb.Record) *Agent {
	str := func(k string) string {
		if v, ok := rec.Data[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	if rec.ID == "" || str("name") == "" {
		return nil
	}
	a := &Agent{
		ID: rec.ID, Owner: owner, Name: str("name"), Kind: str("kind"),
		Prompt: str("prompt"), TokenID: str("token_id"),
	}
	if a.Kind != Hosted {
		a.Kind = External
	}
	if s := str("services"); s != "" {
		a.Services = strings.Split(s, ",")
	}
	if t, err := time.Parse(time.RFC3339, str("created")); err == nil {
		a.Created = t
	} else {
		a.Created = rec.Created
	}
	// Last used comes from the token, which is the only thing that knows: the
	// agent record is not touched by a call.
	if a.TokenID != "" {
		if tok, err := auth.GetTokenByID(a.TokenID); err == nil && tok != nil {
			a.LastUsed = tok.LastUsed
		}
	}
	return a
}

// Endpoint is the MCP URL this agent should be pointed at: scoped, so the tool
// list it reads every turn is its own rather than the whole instance.
func (a *Agent) Endpoint(base string) string {
	if a.Unscoped() {
		return base + "/mcp"
	}
	return base + "/mcp?tools=" + strings.Join(a.Services, ",")
}
