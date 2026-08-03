package api

// Scoping a connection to the tools it actually needs.
//
// One endpoint carrying every tool is the right shape for the server and the
// wrong shape for a session. Sixty-odd tool definitions are sent to the model on
// every turn whether or not any of them could help, and a client that connected
// for news is shown a qibla compass. The criticism on Hacker News was exactly
// this — "you almost never want these tools in any given session" — and it is
// correct.
//
// The fix is not to split the server into nine. It is to let the caller say
// which parts of it they want:
//
//	https://micro.mu/mcp?tools=news,web,mail
//
// The list names services, not tools, because that is the unit a person thinks
// in and the unit the Spec already knows. An unrecognised name is ignored rather
// than refused: a scope is a preference, and failing a whole connection because
// somebody wrote "email" for "mail" would be worse than quietly giving them the
// rest.
//
// Scoping filters what is *listed*, not what may be *called*. A caller who
// knows a tool's name can still use it, and the guards that matter — account,
// credits, rate limits — are unchanged. This is about what an agent is asked to
// consider, which is a context problem, not a permission one.

import (
	"net/http"
	"strings"

	"mu/internal/service"
)

// scopeParam is the query parameter naming the services a connection wants.
const scopeParam = "tools"

// scopeFrom reads the requested services from a request. An empty result means
// no scope: everything is listed, which is what an unscoped URL has always done.
func scopeFrom(r *http.Request) []string {
	if r == nil {
		return nil
	}
	return parseScope(r.URL.Query().Get(scopeParam))
}

// parseScope splits a scope string. Commas or spaces, any case.
func parseScope(raw string) []string {
	fields := strings.FieldsFunc(raw, func(c rune) bool {
		return c == ',' || c == ' ' || c == '+'
	})
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// inScope reports whether a tool belongs to one of the named services.
//
// A tool with no service behind it — agent, chat, quran — is in scope only when
// named directly, so "?tools=news" gets news and nothing else. Naming a tool
// rather than a service works too: someone who wants exactly web_search should
// not have to take web_fetch with it.
func inScope(t Tool, scope []string) bool {
	if len(scope) == 0 {
		return true
	}
	name := strings.ToLower(t.Name)
	// The prefix is read from the name, not from the registry. splitTool only
	// claims a service it can find registered, which would make scoping depend
	// on package initialisation order — working in the binary and silently
	// matching nothing anywhere else. A tool named news_list belongs to news
	// whether or not news has loaded yet.
	svc := ""
	if before, _, ok := strings.Cut(name, "_"); ok {
		svc = before
	}
	for _, want := range scope {
		if want == name || (svc != "" && want == svc) {
			return true
		}
		// The nav label is what the sidebar calls it, and what a person is
		// likely to type: "search" for the web service.
		if svc != "" && strings.EqualFold(want, service.Label(svc)) {
			return true
		}
	}
	return false
}

// ScopeServices lists the service names a caller may scope to, for the UI that
// builds the URL. Derived from the registry, so a new service appears in the
// picker the moment it registers.
func ScopeServices() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range mcpTools() {
		if svc, _ := splitTool(t.Name); svc != "" && !seen[svc] {
			seen[svc] = true
			out = append(out, svc)
		}
	}
	return out
}
