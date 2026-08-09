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
//
// Under agent/ rather than service/, because an agent is not a service. The
// repo has always composed it that way — agent/ sits beside service/ at the top
// level — and the product says the same: you create agents, they call tools,
// and tools are derived from the services underneath. Three levels, and only
// the bottom one belongs in service/. This package spent one day in the wrong
// directory, where the path claimed it was a service while the code registered
// no Spec and the catalogue never listed it.
package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"strconv"

	"mu/agent/micro"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/userdb"
	"mu/service/mail"
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
	// Prompt is the standing instruction: what this agent is for. It is the
	// system prompt when you talk to it.
	Prompt string `json:"prompt,omitempty"`
	// Description is the one-line the router reads when deciding which agent
	// should answer.
	Description string `json:"description,omitempty"`
	// TokenID identifies the credential without holding it. The secret is shown
	// once and never stored in readable form.
	//
	// Empty is a real state: agents created in the chat, and those imported from
	// the store that predates this, have no credential because nobody asked for
	// one. Minting tokens for them at import would have handed out credentials
	// on their owner's behalf.
	TokenID string `json:"token_id,omitempty"`
	// Tag is this agent's own mail address, as the part after the plus in
	// owner+tag@instance.
	//
	// Assigned at creation from the name, because an address you have to think
	// to construct is one nobody constructs. Mu runs a real SMTP server, so an
	// agent here can be written to as well as called — sent a receipt, a form, a
	// reply, a person saying "go ahead" — and that was reachable only by knowing
	// the plus-address convention and inventing a tag by hand.
	//
	// Unique per owner, so two agents cannot read each other's mail.
	Tag string `json:"tag,omitempty"`
	// Public offers this agent to everybody on the instance. What is offered is
	// the recipe — name, description, prompt, tool scope — never the token and
	// never the owner's account: somebody running your agent runs it on their
	// own account, with their own credits, confined to their own scope. Your
	// standing instruction is the product; their account is where it executes.
	//
	// Free, always. Publishing carried a price per question for a while, and a
	// price is a second question on top of an idea people had not yet met — a
	// shared agent already asks you to hold "it runs on my account with their
	// instructions", and "and it costs 5 a go, plus the ordinary query cost"
	// on top of that is where it stopped being explicable. Sharing something
	// useful is the thing worth having first.
	Public bool `json:"public,omitempty"`
	// Runs is what this agent has actually done for other people. A directory
	// with no usage on it is a list of claims.
	Runs     int       `json:"runs,omitempty"`
	ForkedOf string    `json:"forked_of,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitempty"`
}

// Address is the mail address this agent can be written at, or "" if it has no
// tag — agents created before tags existed, which keep working without one.
func (a *Agent) Address() string {
	if a == nil || a.Tag == "" {
		return ""
	}
	return mail.AliasFor(a.Owner, a.Tag)
}

// tagFor makes a mail tag from an agent's name, unique among this owner's
// agents. "Morning Briefer" becomes morningbriefer; a second one becomes
// morningbriefer2, because two agents sharing a tag would share an inbox.
func tagFor(owner, name string, existing []*Agent) string {
	base := mail.CleanTag(strings.ToLower(strings.ReplaceAll(name, " ", "")))
	if base == "" {
		base = "agent"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	taken := map[string]bool{}
	for _, a := range existing {
		if a.Tag != "" {
			taken[a.Tag] = true
		}
	}
	tag := base
	for n := 2; taken[tag]; n++ {
		tag = base + strconv.Itoa(n)
	}
	return tag
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
func CreateAgent(owner, name, kind, prompt, description string, services []string, withToken bool) (*Agent, string, error) {
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
		Owner:       owner,
		Name:        name,
		Kind:        kind,
		Services:    services,
		Prompt:      strings.TrimSpace(prompt),
		Description: strings.TrimSpace(description),
		Tag:         tagFor(owner, name, Agents(owner)),
		Created:     time.Now(),
	}

	// One token per agent, named for it, so the token page and any audit shows
	// which agent a credential belongs to rather than a row called "token 3".
	//
	// Not every agent gets one. An agent you only talk to here has nothing to
	// authenticate, and issuing a credential nobody asked for is how a product
	// ends up with live tokens its owner cannot account for.
	secret := ""
	if withToken {
		tok, s, err := auth.CreateToken(owner, "agent: "+name, auth.ScopeFor(services), time.Time{})
		if err != nil {
			return nil, "", fmt.Errorf("could not issue a token for this agent: %w", err)
		}
		a.TokenID, secret = tok.ID, s
	}

	rec, err := userdb.Create(ns, owner, collection, fields(a), false)
	if err != nil {
		// Do not leave a live credential behind an agent that was not stored.
		if a.TokenID != "" {
			_ = auth.DeleteToken(a.TokenID, owner)
		}
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

// fields is the record a roster agent is stored as. It is hand-built, which
// means every field has to be added here and read back in fromRecord — miss
// either half and the value silently does not persist.
func fields(a *Agent) map[string]any {
	return map[string]any{
		"name": a.Name, "kind": a.Kind, "prompt": a.Prompt,
		"description": a.Description,
		"token_id":    a.TokenID, "services": strings.Join(a.Services, ","),
		"tag":       a.Tag,
		"runs":      a.Runs,
		"forked_of": a.ForkedOf,
		"created":   a.Created.Format(time.RFC3339),
	}
}

// save writes an agent back, carrying its published state. The store's own
// public flag is what makes the directory work: every account's agents live in
// one collection, so a published agent is simply one a stranger's read can see.
func (a *Agent) save() error {
	_, err := userdb.Update(ns, a.Owner, collection, a.ID, fields(a), a.Public)
	return err
}

// EnsureTags gives an address to any agent made before agents had one.
//
// Called when the roster is shown rather than on every read: it writes, and a
// write on a read path is a surprise. One visit to /agents fixes an account, and
// an agent nobody looks at does not need an address yet.
//
// Tags are assigned oldest first so the shortest name wins the unadorned tag,
// rather than whichever agent happened to be loaded first.
func EnsureTags(owner string) {
	all := Agents(owner)
	var missing []*Agent
	for _, a := range all {
		if a.Tag == "" {
			missing = append(missing, a)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Created.Before(missing[j].Created) })
	for _, a := range missing {
		a.Tag = tagFor(owner, a.Name, all)
		_ = a.save()
	}
}

// List returns an owner's agents, newest first.
func Agents(owner string) []*Agent {
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
func AgentFor(owner, id string) *Agent {
	for _, a := range Agents(owner) {
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
func RemoveAgent(owner, id string) error {
	a := AgentFor(owner, id)
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
	num := func(k string) int {
		switch v := rec.Data[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
		return 0
	}
	a := &Agent{
		ID: rec.ID, Owner: owner, Name: str("name"), Kind: str("kind"),
		Prompt: str("prompt"), Description: str("description"), TokenID: str("token_id"),
		Tag: str("tag"), Public: rec.Public,
		Runs: num("runs"), ForkedOf: str("forked_of"),
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

// UpdateAgent rewrites an agent the owner owns, keeping its id and token.
func UpdateAgent(owner, id, name, prompt, description string, services []string) (*Agent, error) {
	a := AgentFor(owner, id)
	if a == nil {
		return nil, fmt.Errorf("no such agent")
	}
	if n := strings.TrimSpace(name); n != "" {
		a.Name = n
	}
	a.Prompt = strings.TrimSpace(prompt)
	a.Description = strings.TrimSpace(description)
	a.Services = validServices(services)
	// a.save(), not a bare Update with public:false — editing an agent must not
	// silently unpublish it.
	if err := a.save(); err != nil {
		return nil, err
	}
	return a, nil
}

// IssueToken gives an agent a credential, replacing any it had.
//
// Separate from creation because an agent you only talked to may later need to
// run somewhere else, and because the secret can only be shown once — so it has
// to be an action somebody takes deliberately rather than a side effect.
func IssueToken(owner, id string) (string, error) {
	a := AgentFor(owner, id)
	if a == nil {
		return "", fmt.Errorf("no such agent")
	}
	if a.TokenID != "" {
		_ = auth.DeleteToken(a.TokenID, owner)
	}
	tok, secret, err := auth.CreateToken(owner, "agent: "+a.Name, auth.ScopeFor(a.Services), time.Time{})
	if err != nil {
		return "", err
	}
	a.TokenID = tok.ID
	if err := a.save(); err != nil {
		return "", err
	}
	return secret, nil
}

// ImportUserAgents moves agents from the store that predates the roster.
//
// There were two stores: this one, and agent/micro's, which the chat wrote to.
// Two creation paths writing to different places meant "my agents" depended on
// which page you asked. Imported agents keep their name, prompt, description and
// tool set, and get no token — nobody asked for a credential, and minting one on
// somebody's behalf is not an import, it is a decision.
//
// Idempotent by name: running twice does not double anybody's list.
func ImportUserAgents(all map[string][]*micro.Agent) int {
	imported := 0
	for owner, list := range all {
		have := map[string]bool{}
		for _, a := range Agents(owner) {
			have[strings.ToLower(a.Name)] = true
		}
		for _, ua := range list {
			if ua == nil || have[strings.ToLower(ua.Name)] {
				continue
			}
			if _, _, err := CreateAgent(owner, ua.Name, Hosted, ua.SystemPrompt,
				ua.Description, ua.Tools, false); err != nil {
				continue
			}
			have[strings.ToLower(ua.Name)] = true
			imported++
		}
	}
	return imported
}

// AsMicro presents a roster agent as one the chat can run: its prompt becomes
// the system prompt, and its scope becomes the tool set, which is what the
// native path filters services by.
func (a *Agent) AsMicro() *micro.Agent {
	desc := a.Description
	if desc == "" {
		desc = a.Prompt
	}
	return &micro.Agent{
		ID: a.ID, Name: a.Name, Description: desc,
		SystemPrompt: a.Prompt, Tools: a.Services, OwnerAccountID: a.Owner,
	}
}

// ── Sharing ─────────────────────────────────────────────────────
//
// What is shared is the recipe, never the account.
//
// An app is inert HTML, so running somebody's app is just rendering it. An
// agent calls tools, and whose tools it calls is the whole question. So a
// published agent runs on the *asker's* account: their credits pay for the
// model and the tool calls, their scope bounds it, their mail is what mail
// tools reach. What comes from the author is the standing instruction and the
// tool list — the part that took thought. Nothing else crosses, and in
// particular the author's token never does.
//
// It is free, deliberately. Publishing carried a price per question for a
// while, and a price is a second unfamiliar idea stacked on the first: a shared
// agent already asks you to hold "it runs on my account, with their
// instructions", and "and it costs 5 a go, on top of the ordinary query cost"
// is where that stopped being explicable. Sharing something useful is worth
// having on its own; charging for it is a separate question for a later day.

// PublicAgents returns every agent published on this instance, most used first.
// Naming it a directory would overstate it: it is one collection, read with the
// store's own public scope.
func PublicAgents(viewer string) []*Agent {
	records, err := userdb.List(ns, viewer, collection, "public", nil, "", "", 200)
	if err != nil {
		return nil
	}
	var out []*Agent
	for i := range records {
		if a := fromRecord(records[i].Owner, records[i]); a != nil && a.Public {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Runs != out[j].Runs {
			return out[i].Runs > out[j].Runs
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// PublicAgent returns one published agent by id, whoever owns it.
func PublicAgent(viewer, id string) *Agent {
	for _, a := range PublicAgents(viewer) {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// Publish offers an agent to everybody, or withdraws it.
//
// An agent with no standing instruction has nothing to offer — the recipe is
// the prompt, so publishing an empty one puts a name in a list and gives
// whoever runs it the default assistant.
func Publish(owner, id string, public bool) error {
	a := AgentFor(owner, id)
	if a == nil {
		return fmt.Errorf("no such agent")
	}
	if public && strings.TrimSpace(a.Prompt) == "" {
		return fmt.Errorf("give it a system prompt first — that is what you are sharing")
	}
	a.Public = public
	return a.save()
}

// Fork copies a published agent into your own roster, so you can change it.
func Fork(viewer, id string) (*Agent, error) {
	src := PublicAgent(viewer, id)
	if src == nil {
		return nil, fmt.Errorf("no such agent")
	}
	if src.Owner == viewer {
		return nil, fmt.Errorf("that one is already yours")
	}
	name := src.Name
	have := map[string]bool{}
	for _, a := range Agents(viewer) {
		have[strings.ToLower(a.Name)] = true
	}
	for i := 2; have[strings.ToLower(name)]; i++ {
		name = fmt.Sprintf("%s %d", src.Name, i)
	}
	// Hosted and tokenless: a copy is something you talk to until you decide
	// otherwise, and minting a credential on somebody's behalf is a decision.
	a, _, err := CreateAgent(viewer, name, Hosted, src.Prompt, src.Description, src.Services, false)
	if err != nil {
		return nil, err
	}
	a.ForkedOf = src.ID
	_ = a.save()
	return a, nil
}

// RunPublic resolves a published agent for somebody who does not own it, and
// counts the run.
//
// Returns nil for an agent that is not published, which is what makes an id
// alone useless: knowing the id of a private agent gets you the default
// assistant, the same as knowing nothing.
func RunPublic(asker, id string) *Agent {
	a := PublicAgent(asker, id)
	if a == nil || a.Owner == asker {
		return nil
	}
	a.Runs++
	_ = a.save()
	return a
}

// AgentForTag finds an agent by the mail tag it answers on: the part after the
// plus in you+<tag>@. Empty tag, unknown tag, or an agent made before tags
// existed all return nil, so mail to a tag that is not an agent — you+receipts@
// — just files in the inbox like any other tagged mail.
func AgentForTag(owner, tag string) *Agent {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if owner == "" || tag == "" {
		return nil
	}
	for _, a := range Agents(owner) {
		if strings.EqualFold(a.Tag, tag) {
			return a
		}
	}
	return nil
}
