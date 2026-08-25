package api

import (
	"encoding/json"
	"strings"

	"mu/internal/service"
)

// Tool annotations: the metadata a client needs to decide how to treat a tool
// before it calls one.
//
// A read is safe to retry and safe to run without asking; a write is not; an
// irreversible write should be confirmed. Clients render these as badges and
// confirmation prompts, and the Anthropic Connectors Directory checks for them
// at submission. Mu published none of it, while already knowing all of it:
// service.Destructive comes from each endpoint's own declaration, and the verb
// in a tool name is the convention this codebase already enforces
// (TestNoMethodRepeatsItsService).
//
// So this derives rather than declares. Nothing here is a new list to keep in
// step with the old ones — which is the failure mode the Spec exists to end.

// mcpAnnotations is the MCP tool annotations object.
type mcpAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
}

// readVerbs name a method that returns something and changes nothing. The list
// is of verbs, not tools, so a new service inherits the classification.
var readVerbs = map[string]bool{
	"list": true, "get": true, "read": true, "search": true, "inbox": true,
	"address": true,
	"nearby":  true, "forecast": true, "balance": true, "today": true,
	"prayer": true, "qibla": true, "eta": true, "find": true, "free": true,
}

// writeVerbs name a method that creates or changes something.
var writeVerbs = map[string]bool{
	"create": true, "send": true, "post": true, "update": true, "edit": true,
	"build": true, "generate": true, "run": true, "fork": true, "save": true,
	"put": true, "share": true,
	"transfer": true, "topup": true, "add": true, "schedule": true,
}

// removeVerbs name a method whose effect cannot be undone.
var removeVerbs = map[string]bool{
	"delete": true, "remove": true, "unsave": true, "dismiss": true,
	"cancel": true, "block": true,
}

// wholeTitles are verbs whose label is a complete title on its own, where
// appending the service name reads worse rather than better.
var wholeTitles = map[string]string{
	"eta":     "Travel time",
	"free":    "Find free time",
	"address": "Get an email address",
}

// verbLabels give a read verb a human phrasing for the title. "list" reads
// better as "Browse" beside a service name, "get" as "Read".
var verbLabels = map[string]string{
	"list": "Browse", "get": "Read", "read": "Read", "search": "Search",
	"inbox": "Read", "nearby": "Find nearby", "forecast": "Forecast",
	"balance": "Check", "create": "Create", "send": "Send", "post": "Post",
	"update": "Update", "edit": "Edit", "build": "Build", "generate": "Generate",
	"run": "Run", "delete": "Delete", "fork": "Fork", "put": "Store",
	"share":    "Share",
	"transfer": "Transfer", "today": "Today's", "prayer": "Prayer times for",
	"qibla": "Qibla for", "free": "Free time in", "find": "Find",
}

// annotate derives a tool's annotations from its name and its service's own
// declaration.
func annotate(t Tool) *mcpAnnotations {
	svc, verb := splitTool(t.Name)

	a := &mcpAnnotations{Title: toolTitle(t, svc, verb)}

	switch {
	case removeVerbs[verb] || service.Destructive(svc, verb):
		a.DestructiveHint = true
	case readVerbs[verb] && !writeVerbs[verb]:
		a.ReadOnlyHint = true
		// A read returns the same thing for the same arguments, so a client may
		// safely repeat one it did not see the answer to.
		a.IdempotentHint = true
	}
	return a
}

// splitTool separates a tool name into its service and its verb.
//
// The service is only claimed when it is registered, because it is used to look
// up that service's own declaration. The verb is found from the name alone, and
// deliberately so: an earlier version required the service to be registered
// before it would classify anything, which made a tool's annotations depend on
// package initialisation order — present in the running binary, absent in a
// test, and silently absent for any tool whose service had not loaded yet.
// Whether a call reads or destroys is not a fact that should come and go.
//
// Most names are service_method (news_list). A few are verb_noun (block_user,
// unblock_user), and a few are a bare word (agent, hadith).
func splitTool(name string) (svc, verb string) {
	name = strings.ToLower(strings.TrimSpace(name))
	parts := strings.Split(name, "_")

	if len(parts) > 1 {
		if _, known := service.SpecFor(parts[0]); known {
			svc = parts[0]
		}
	}

	known := func(s string) bool {
		return readVerbs[s] || writeVerbs[s] || removeVerbs[s]
	}
	// service_method puts the verb last; verb_noun puts it first.
	if last := parts[len(parts)-1]; known(last) {
		return svc, last
	}
	if known(parts[0]) {
		return svc, parts[0]
	}
	if svc != "" {
		return svc, strings.Join(parts[1:], "_")
	}
	return "", name
}

// toolTitle is the short human label a client shows instead of the raw name.
//
// It is built from the verb and the service's own nav label, so a tool says the
// same word a person sees in the sidebar and there is no second name to
// maintain. Tool.Title is deliberately not consulted: it is a caller-supplied
// noun where this wants the verb phrase that names an action.
func toolTitle(t Tool, svc, verb string) string {
	if svc == "" {
		// Bare names (agent, hadith) title from themselves; a two-part name with
		// no service behind it (saved_list) reads better whole than as its verb.
		return humanise(t.Name)
	}

	// A few verbs are already a whole title; "Time to places" is worse than
	// "Travel time".
	if whole, ok := wholeTitles[verb]; ok {
		return whole
	}

	label := strings.ToLower(service.Label(svc))
	lead, ok := verbLabels[verb]
	if !ok {
		lead = humanise(verb)
	}
	// The web service is labelled "Search", so the verb and the label are the
	// same word and "Search search" is what falls out. Name the service instead:
	// "Search the web".
	if strings.EqualFold(lead, label) {
		return lead + " the " + svc
	}
	return lead + " " + label
}

// enrichToolsList adds title and annotations to a tools/list response.
//
// It is done to the rendered JSON rather than to the tool type because the tool
// type belongs to go-micro, whose gateway/mcp.Tool carries no annotations
// field. Adding one there to satisfy Mu is exactly the dependency direction
// AGENTS.md rules out: go-micro changes when framework users need something,
// never because Mu does. So the protocol stays the framework's and this stays
// ours.
//
// Anything unexpected is passed through untouched — a tools/list that does not
// parse is still a valid response to return, and dropping it to add a badge
// would be a poor trade.
func enrichToolsList(body []byte) []byte {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		return body
	}
	listed, ok := result["tools"].([]any)
	if !ok {
		return body
	}

	known := map[string]Tool{}
	for _, t := range mcpTools() {
		known[t.Name] = t
	}

	for _, item := range listed {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		t, found := known[name]
		if !found {
			t = Tool{Name: name}
		}
		a := annotate(t)
		if a.Title != "" {
			entry["title"] = a.Title
		}
		entry["annotations"] = a
	}

	out, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return out
}

// humanise turns a bare tool name into a label: "quran_search" never reaches
// here, but "agent", "hadith" and "saved_list" do.
func humanise(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
