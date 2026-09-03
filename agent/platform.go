package agent

// The agents this instance offers, as opposed to the ones an account made.
//
// Two namespaces, and the split is the address:
//
//	you+research@<domain>   your own agent, in your own namespace
//	agent+news@<domain>     this instance's, in the platform's
//
// "agent" is a reserved username, so agent+<name>@ is the instance speaking for
// itself and cannot collide with anybody's roster. That is what makes the two
// safe to keep side by side: a name means one thing in your namespace and
// another in the platform's, and the local part says which you meant. Without
// the split, adding a built-in agent could silently take over an address
// somebody was already using.
//
// The list is the micro registry — the same eleven specialists the router picks
// between — so an agent is addressable by mail the moment it is registered, and
// nothing here has to be kept in step by hand.

import (
	"sort"
	"strings"

	"mu/agent/micro"
	"mu/service/mail"
)

// DefaultPlatformAgent is who answers the untagged address. Micro is the
// catch-all with every tool, which is what somebody writing to agent@ for the
// first time means.
const DefaultPlatformAgent = "micro"

// DefaultName is what the agent that answers by default is called.
//
// The display name and not the id, for the places that put it on screen: the
// byline over a reply on Home and on the front door, where the reader has
// picked nobody and Micro is who answers. Read from the registry rather than
// written out, so an instance that renames or replaces its default agent says
// the new name everywhere without a second edit.
func DefaultName() string {
	if a := micro.Get(DefaultPlatformAgent); a != nil && strings.TrimSpace(a.Name) != "" {
		return a.Name
	}
	return ""
}

// Platform resolves a name on the shared address to one of this instance's
// agents. Empty names the default. Nil when it names nothing.
func Platform(name string) *micro.Agent {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = DefaultPlatformAgent
	}
	return micro.Get(name)
}

// PlatformNames is every name reachable at agent+<name>@, sorted, with the
// default first — it is the one to try, so it should be the one read first.
//
// Used to answer somebody who wrote to a name that is not here. A list of what
// does work is the only useful thing to say to a typo.
func PlatformNames() []string {
	var rest []string
	for id := range micro.Registry {
		if id != DefaultPlatformAgent {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	if micro.Get(DefaultPlatformAgent) == nil {
		return rest
	}
	return append([]string{DefaultPlatformAgent}, rest...)
}

// PlatformOpts is how to run one: its own prompt and its own tools.
//
// A specialist with every tool is not a specialist — the point of writing to
// agent+markets@ rather than agent@ is a narrower thing that answers about
// prices, so the allow-list travels with it. Nil tools means all, which is what
// the catch-all wants.
func PlatformOpts(a *micro.Agent) QueryOpts {
	if a == nil {
		return QueryOpts{}
	}
	return QueryOpts{System: a.SystemPrompt, Tools: a.Tools, Model: a.Model}
}

// platformName is a display name for a run record, so the runs page can say
// which agent answered without knowing that the platform has any.
func platformName(id string) string {
	if a := micro.Get(id); a != nil {
		return a.Name
	}
	return ""
}

// PlatformNamed resolves a name to one of this instance's agents and reports
// its id and display name, or false if there is no such agent.
//
// Two strings rather than the agent itself, because the callers are clients and
// a client has no business importing the registry: naming the type meant
// agent/mail imported agent/micro to hold a variable. What a client needs is
// which agent answered and what to call it.
func PlatformNamed(name string) (id, display string, ok bool) {
	a := Platform(name)
	if a == nil {
		return "", "", false
	}
	return a.ID, a.Name, true
}

// examplesFor is what is worth asking this agent, for the box on its page.
//
// No account argument, and the test above this file's rules is why: platform
// resolution means the same thing to everybody. Only the platform's own agents
// have examples — one somebody made themselves has a name and an instruction
// and no examples, and inventing four from its prompt would be a guess
// presented as the agent's own words. The general set is the honest fallback.
func examplesFor(agentID string) []string {
	if agentID == "" {
		return nil
	}
	if a := micro.Get(agentID); a != nil {
		return a.Examples
	}
	return nil
}

// PlatformAddress is where one of this instance's own agents is written to.
//
// agent+news@ for the specialists, and the bare agent@ for the default —
// because the default is what an untagged message already reaches, so
// agent+micro@ is the same mailbox spelled longer. It was published that way on
// its own Connect page: "Connect to Micro / agent+micro@micro.mu", naming a tag
// whose only effect is to select the agent you get anyway.
//
// Here rather than in service/mail, because which agent is the default is this
// package's fact and a service may not import an agent. Four call sites were
// each deciding it for themselves, and three of them got it right.
func PlatformAddress(id string) string {
	if strings.EqualFold(id, DefaultPlatformAgent) {
		id = ""
	}
	return mail.SharedAgentAddressFor(id)
}
