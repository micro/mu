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
)

// DefaultPlatformAgent is who answers the untagged address. Micro is the
// catch-all with every tool, which is what somebody writing to agent@ for the
// first time means.
const DefaultPlatformAgent = "micro"

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
	return QueryOpts{System: a.SystemPrompt, Tools: a.Tools}
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
// client/mail imported agent/micro to hold a variable. What a client needs is
// which agent answered and what to call it.
func PlatformNamed(name string) (id, display string, ok bool) {
	a := Platform(name)
	if a == nil {
		return "", "", false
	}
	return a.ID, a.Name, true
}
