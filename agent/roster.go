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
// agents with it. Every agent runs here — see Kind below for the distinction
// this used to make and why it stopped making it.
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
	"mu/internal/ai"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/userdb"
	"mu/service/mail"
)

const (
	ns         = "agents"
	collection = "agents"
)

// Kind is a field on every stored record and a distinction the product no
// longer makes.
//
// There were two: Hosted, meaning this instance executes the standing
// instruction, and External, meaning the agent runs in Claude or Cursor and
// calls in with its token. The create form asked which before you had written
// anything, and the question contradicted the form it was on — an agent running
// in Cursor does not use the prompt you are typing, so half the page configured
// something the other half had declared irrelevant.
//
// It is one thing now. Every agent runs here, answers at POST /agent/<name> and
// has a page you can chat on; a token is something you additionally hand to a
// program outside, which is a row on the Connect page rather than a species of
// agent. Nothing branches on this any more.
//
// The constants stay because records on disk carry the word, and normalising
// them on read is cheaper than a migration that has nothing to migrate to.
const (
	Hosted   = "hosted"
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
	// Model is which model this agent answers with. Empty is the instance's
	// default, and is what every agent made before this field existed has.
	//
	// The plumbing was already whole: QueryOpts.Model reaches nativeLLMFor,
	// which picks the provider that serves the named id. Only the built-in
	// agents ever set it, so "use the cheap one for this job" was a choice
	// Micro and Code had and nobody's own agent did.
	//
	// Validated against ai.Choices when it is set rather than when it is used.
	// A model no provider here serves fails at the model call, minutes later,
	// on a run somebody is waiting for — and the person who could fix it is
	// the one who typed it, at the moment they typed it.
	Model string `json:"model,omitempty"`
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
	//
	// Follows the name. It used to be assigned once and never revisited, so
	// renaming an agent moved the label and left the address, the page and the
	// endpoint on the old word — see Rename.
	Tag string `json:"tag,omitempty"`
	// Former is the tags this agent used to answer to, oldest first.
	//
	// A tag is an address, and the whole point of renaming it with the name is
	// that the visible name and the address agree. But the old one is written
	// down somewhere by then — a cron job, a saved curl, a contact card, mail
	// already in flight — and changing an address out from under those is how a
	// rename becomes an outage. So the old ones keep resolving.
	//
	// A live tag always wins over a former one, so reusing a name another agent
	// has since taken cannot hijack it.
	Former   []string  `json:"former,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitempty"`

	// There were three more: Public, Runs and ForkedOf, for publishing an agent
	// to everybody on the instance and running somebody else's. It was a
	// marketplace with no sellers — Fork came off the roster page a while back
	// for offering to copy an agent to people who had never made one, and
	// nothing else ever linked to the directory, so PublicAgents, PublicAgent,
	// Publish and Fork had no callers at all. What is left of a directory
	// nobody can reach is a public flag on a record and a counter that only
	// counts.
	//
	// Sharing an agent is a real idea and this is not an argument against it.
	// It is an argument against keeping the half of it that shipped: a recipe
	// somebody can run on their own account is worth building when there is a
	// page to find one on.
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

// AgentAllowance answers how many agents this account may keep, and is filled
// in by internal/server/hooks.go.
//
// It used to be what a plan allowed. Plans are gone; the answer now comes from
// how accountable an account is — a verified address or money on the balance —
// which is the thing the cap was standing in for all along.
//
// Nil on a build with no billing linked in, which is what a self-hosted
// instance is: nobody is selling anything there, so nobody is limited.
var AgentAllowance func(accountID string) int

// agentAllowance is how many this account may keep, or 0 for no limit.
func agentAllowance(owner string) int {
	if AgentAllowance == nil {
		return 0
	}
	return AgentAllowance(owner)
}

// AtAgentLimit reports whether this account may not make another agent, with
// how many it has and how many it may have.
//
// Asked before the door rather than behind it. The limit was checked only where
// an agent is actually made, so the way to find out was to press New agent,
// fill in a name, a description, a prompt and a set of tools, submit, and be
// told no — every part of which had to be typed first. A limit is not a
// surprise to spring on somebody who has already done the work.
func AtAgentLimit(owner string) (full bool, have, max int) {
	max = agentAllowance(owner)
	have = len(Agents(owner))
	return max > 0 && have >= max, have, max
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

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
	// Everything runs here. The parameter is kept because callers pass it and
	// records on disk carry it; there is nothing left to decide. See Kind.
	kind = Hosted

	// How many agents an account may keep is what its plan sells. The pricing
	// page has offered 1, 5 and 25 since it was written and nothing counted
	// them, so every account had as many as it liked and the first line of
	// every card was decoration.
	//
	// Checked here because this is the only place an agent is made. The message
	// says the number and where to change it, because a limit somebody meets
	// without being told what it is reads as a bug.
	existing := Agents(owner)
	if max := agentAllowance(owner); max > 0 && len(existing) >= max {
		return nil, "", fmt.Errorf("your plan runs %d agent%s and you have %d — "+
			"verify your address or add credit to your account to run more, or delete one first",
			max, plural(max), len(existing))
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
		"description": a.Description, "model": a.Model,
		"token_id": a.TokenID, "services": strings.Join(a.Services, ","),
		"tag":     a.Tag,
		"former":  strings.Join(a.Former, ","),
		"created": a.Created.Format(time.RFC3339),
	}
}

// save writes an agent back, carrying its published state. The store's own
// public flag is what makes the directory work: every account's agents live in
// one collection, so a published agent is simply one a stranger's read can see.
func (a *Agent) save() error {
	_, err := userdb.Update(ns, a.Owner, collection, a.ID, fields(a), false)
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
func For(owner, id string) *Agent {
	for _, a := range Agents(owner) {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// NameOf is what to call an agent when all you have is its id.
//
// Three places an id can come from, and a caller holding one out of the record
// has no way to know which: the roster, the store the roster replaced, and this
// instance's own agents, whose ids are names already. Asking only the first is
// what put raw ids in front of people — the inbox draws a mailbox per agent, and
// three of its four boxes were labelled with a uuid.
//
// Empty when it resolves to nothing, and that is a real answer rather than a
// failure: an id that names no agent means the agent is gone. A caller should
// say nothing about it, not print the id — which is a name for a row in a file,
// not for anything a person has met.
func NameOf(owner, id string) string {
	if strings.TrimSpace(id) == "" {
		return ""
	}
	if a := For(owner, id); a != nil && a.Name != "" {
		return a.Name
	}
	if a := micro.UserAgentFor(owner, id); a != nil && a.Name != "" {
		return a.Name
	}
	return platformName(id)
}

// Remove deletes an agent and revokes its token.
//
// Both, always. An agent removed from the list while its credential kept
// working would be the worst of both: gone from the page that would have told
// you it existed, and still able to call.
func RemoveAgent(owner, id string) error {
	a := For(owner, id)
	if a == nil {
		return fmt.Errorf("no such agent")
	}
	if a.TokenID != "" {
		_ = auth.DeleteToken(a.TokenID, owner)
	}
	// Also from the store this one replaced, matched the way the importer
	// matches: by name.
	//
	// Deleting an agent did not stick. The roster record went, and the copy in
	// agent/micro's store stayed, and ImportUserAgents ran on every startup —
	// so the agent came back at the next restart, and the next, and the next.
	// Removing it from both is what makes a deletion a deletion; the ids differ
	// because importing minted a new one, so the name is the only handle there
	// has ever been.
	forgetLegacyAgent(owner, a.Name)
	return userdb.Delete(ns, owner, collection, id)
}

// forgetLegacyAgent drops any pre-roster record of this name.
func forgetLegacyAgent(owner, name string) {
	want := strings.ToLower(strings.TrimSpace(name))
	if owner == "" || want == "" {
		return
	}
	for _, ua := range micro.UserAgentsFor(owner) {
		if ua != nil && strings.ToLower(ua.Name) == want {
			micro.DeleteUserAgentFor(owner, ua.ID)
		}
	}
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
		Prompt: str("prompt"), Description: str("description"), TokenID: str("token_id"),
		Tag: str("tag"), Model: str("model"),
	}
	// Records written before the distinction went carry kind:"external". They
	// are ordinary agents: normalised on read, so nothing downstream has to know
	// the word ever existed.
	a.Kind = Hosted
	if s := str("services"); s != "" {
		a.Services = strings.Split(s, ",")
	}
	if s := str("former"); s != "" {
		a.Former = strings.Split(s, ",")
	}
	if t, err := time.Parse(time.RFC3339, str("created")); err == nil {
		a.Created = t
	} else {
		a.Created = rec.Created
	}
	// Last used comes from the token, which is the only thing that knows: the
	// agent record is not touched by a call.
	if a.TokenID != "" {
		if tok, err := auth.TokenByID(a.TokenID); err == nil && tok != nil {
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

// formerLimit caps how many old tags one agent carries.
//
// Renaming is rare and each entry is a few bytes, but nothing stops somebody
// renaming an agent a thousand times, and an unbounded list on a record read on
// every page is a slow leak rather than a bug anybody notices. The oldest go
// first: a tag five renames ago is not the one anybody has written down.
const formerLimit = 8

// retag moves an agent onto the tag its new name implies, keeping the old one
// resolving.
//
// Nothing happens when the name produces the tag the agent already has, which
// is the common case — editing the prompt, changing the tool scope, fixing a
// typo in the description all arrive here and must not churn the address.
func (a *Agent) retag(all []*Agent) {
	// Every name already spoken for by one of this owner's other agents. Self is
	// excluded, or an agent would rename itself to research2. Their former tags
	// count as taken: an old link to somebody else's old name still resolves,
	// and handing that word to this agent would make it ambiguous.
	var taken []*Agent
	for _, o := range all {
		if o.ID == a.ID {
			continue
		}
		taken = append(taken, o)
		for _, f := range o.Former {
			taken = append(taken, &Agent{Tag: f})
		}
	}
	next := tagFor(a.Owner, a.Name, taken)
	if next == a.Tag || next == "" {
		return
	}
	old := a.Tag
	a.Tag = next
	// Reclaiming a name this agent used before: it is live again, so it is not
	// also a former one.
	a.Former = without(a.Former, next)
	if old != "" {
		a.Former = append(a.Former, old)
	}
	if n := len(a.Former); n > formerLimit {
		a.Former = a.Former[n-formerLimit:]
	}
}

// without returns list with tag removed.
func without(list []string, tag string) []string {
	out := list[:0:0]
	for _, s := range list {
		if !strings.EqualFold(s, tag) {
			out = append(out, s)
		}
	}
	return out
}

// UpdateAgent rewrites an agent the owner owns, keeping its id and token.
func UpdateAgent(owner, id, name, prompt, description string, services []string) (*Agent, error) {
	all := Agents(owner)
	var a *Agent
	for _, x := range all {
		if x.ID == id {
			a = x
			break
		}
	}
	if a == nil {
		return nil, fmt.Errorf("no such agent")
	}
	if n := strings.TrimSpace(name); n != "" {
		a.Name = n
		// The tag follows the name, so the address, the page and the endpoint
		// say what the agent is called. Renaming used to move the label alone:
		// /agents said "Research" and the thing you were told to POST to was
		// still /agent/test.
		a.retag(all)
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

// SetModel chooses which model an agent answers with. Empty clears it, and an
// agent with no model answers on the instance's default.
//
// Its own function rather than a seventh string on UpdateAgent. That signature
// already carries owner, id, name, prompt and description in a row, and the
// bug an eighth invites — two of them swapped at a call site, compiling
// perfectly — costs more than a second write. Rename is separate for the same
// reason.
//
// Refused here rather than at the model call. A model no provider on this
// instance serves fails minutes later, on a run somebody is waiting for, and
// the person who could fix it is the one who chose it — at the moment they
// chose it. See ai.Offered.
func SetModel(owner, id, model string) error {
	a := For(owner, id)
	if a == nil {
		return fmt.Errorf("no such agent")
	}
	model = strings.TrimSpace(model)
	if !ai.Offered(model) {
		return fmt.Errorf("this instance cannot run %q — it has no key for a provider that serves it", model)
	}
	a.Model = model
	return a.save()
}

// IssueToken gives an agent a credential, replacing any it had.
//
// Separate from creation because an agent you only talked to may later need to
// run somewhere else, and because the secret can only be shown once — so it has
// to be an action somebody takes deliberately rather than a side effect.
func IssueToken(owner, id string) (string, error) {
	a := For(owner, id)
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
// It moves rather than copies: every agent it reads is removed from the old
// store, whether it was imported or skipped for already being in the roster.
//
// This ran on every startup and only copied, so the old store stayed full
// forever and the import was not a migration, it was a sync from a deprecated
// source. That made deletion impossible: remove an agent and the next restart
// imported it straight back from the copy nobody could see. Draining the source
// is what makes the migration a migration — after one pass there is nothing
// left to resurrect anything from.
func ImportUserAgents(all map[string][]*micro.Agent) int {
	imported := 0
	for owner, list := range all {
		have := map[string]bool{}
		for _, a := range Agents(owner) {
			have[strings.ToLower(a.Name)] = true
		}
		for _, ua := range list {
			if ua == nil {
				continue
			}
			name := strings.ToLower(ua.Name)
			if have[name] {
				// Already in the roster: it was imported on an earlier pass, so
				// the old copy is the stale one.
				micro.DeleteUserAgentFor(owner, ua.ID)
				continue
			}
			if _, _, err := CreateAgent(owner, ua.Name, Hosted, ua.SystemPrompt,
				ua.Description, ua.Tools, false); err != nil {
				continue // left in place, so the next run tries again
			}
			micro.DeleteUserAgentFor(owner, ua.ID)
			have[name] = true
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
		Model: a.Model,
	}
}

// ForTag finds an agent by the mail tag it answers on: the part after the
// plus in you+<tag>@. Empty tag, unknown tag, or an agent made before tags
// existed all return nil, so mail to a tag that is not an agent — you+receipts@
// — just files in the inbox like any other tagged mail.
func ForTag(owner, tag string) *Agent {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if owner == "" || tag == "" {
		return nil
	}
	all := Agents(owner)
	for _, a := range all {
		if strings.EqualFold(a.Tag, tag) {
			return a
		}
	}
	// Then a name it used to answer to, so mail addressed before a rename — or
	// sent by somebody working from an old contact card — still arrives. A live
	// tag is checked first, above, so an agent that took the word since cannot
	// be shadowed by whoever had it before.
	for _, a := range all {
		for _, f := range a.Former {
			if strings.EqualFold(f, tag) {
				return a
			}
		}
	}
	return nil
}
