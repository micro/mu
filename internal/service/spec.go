package service

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	"go-micro.dev/v6/server"
)

// Spec is everything Mu knows about a service, declared once, in the service's
// own package, and read by every surface.
//
//	service.Register(service.Spec{
//		Name:        "web",
//		Handler:     new(Server),
//		Description: "The open web: search it, read a page from it",
//		Page:        "/search",
//		Label:       "Search",
//		Endpoints: map[string]service.Endpoint{
//			"Search": {Doc: "Search the web…", Cost: wallet.OpWebSearch},
//			"Fetch":  {Doc: "Fetch a web page…", Cost: wallet.OpWebFetch},
//		},
//	})
//
// Before this there were fourteen places a service was declared — the registry,
// two MCP tool lists, the REST reference, the nav, two guest allowlists, the
// agent's labels, the destructive list, the account-scoped list, the pricing
// ops, the micro-agent tool lists and two documentation tables. Each was
// correct on its own and none knew about the others, which is how the same
// capability ended up called search, search_web, index and web_search at once,
// and how Stream and Chat came to share an icon.
//
// The rule is that a surface may read this and derive what it needs. It may not
// keep its own list. Anything a surface cannot derive belongs here as a field.
type Spec struct {
	// Name is the service name — the directory, the route, the tool prefix.
	Name string
	// Handler is the go-micro handler whose exported methods become endpoints.
	Handler any
	// Description says what the service is, for humans: status, docs, the
	// service list. Endpoint docs say what each method does, for a model.
	Description string
	// Page is the web route, e.g. "/news". Empty means headless: a capability
	// with no page, reached only by the agent, apps and other services.
	Page string
	// Label is the nav label. Defaults to the name, title-cased. It differs
	// from the name when the human word and the domain word differ — the web
	// service is "Search" in the sidebar, because that is what a person looks
	// for.
	Label string
	// Icon is the sidebar icon file, e.g. "news.png". Defaults to
	// "<name>.svg". It lives here because it is part of what the service is:
	// Stream and Chat shared a speech bubble for months because the nav was a
	// hand-written list and nothing could notice the repeat.
	Icon string
	// Scoped marks a service holding one user's data, or spending their
	// credits. A caller with no authenticated account cannot reach it at all.
	Scoped bool
	// Endpoints describes each method, keyed by method name. Every exported
	// RPC method must appear; TestEveryEndpointIsDescribed enforces it.
	Endpoints map[string]Endpoint
	// Card renders this service at a glance: the markets table, today's
	// headlines, the forecast. Nil means the service has nothing to show.
	//
	// These renderers already existed and were reachable from exactly one
	// place — the home screen, through a map of names kept by hand beside the
	// services it named. Declared here they are derivable, which is what lets
	// anything else use them: a page, an app, an agent answering "what are
	// markets doing" with the card instead of a paragraph.
	//
	// A card is a view, not a widget. It renders and it links; it does not
	// hold state or take input. Anything that does belongs in an app, which
	// already has a sandbox and a security boundary — there is no reason for a
	// second one.
	Card func() string
}

// Endpoint is one method of a service.
type Endpoint struct {
	// Doc is what the method does, written for a model rather than a
	// developer: it is what the agent reads when choosing a tool.
	Doc string
	// Cost is the wallet operation charged per call (wallet.Op…). Empty means
	// the call is free — it touches only this instance's own storage.
	Cost string
	// Destructive withholds the method from the model. The agent reads
	// attacker-controlled text, so a tool it holds is a tool prompt injection
	// holds; what earns this flag is an irreversible effect nobody asked for.
	Destructive bool
}

// Tool is the derived tool name for a method: service_method, lower-cased.
// Deriving it is why a service is named for a domain and a method for an
// action — see TestNoMethodRepeatsItsService.
func (s Spec) Tool(method string) string {
	return s.Name + "_" + strings.ToLower(method)
}

// NavLabel is the label, defaulting to the title-cased name.
func (s Spec) NavLabel() string {
	if s.Label != "" {
		return s.Label
	}
	if s.Name == "" {
		return ""
	}
	return strings.ToUpper(s.Name[:1]) + s.Name[1:]
}

// NavIcon is the icon file, defaulting to "<name>.svg".
func (s Spec) NavIcon() string {
	if s.Icon != "" {
		return s.Icon
	}
	return s.Name + ".svg"
}

// Headless reports whether the service has no page of its own.
func (s Spec) Headless() bool { return s.Page == "" }

// Cards returns the services that can render themselves at a glance, ordered
// by label.
func Cards() []Spec {
	out := make([]Spec, 0, 8)
	for _, s := range Specs() {
		if s.Card != nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NavLabel() < out[j].NavLabel() })
	return out
}

// CardFor renders one service's card, or "" if it has none.
func CardFor(name string) string {
	s, ok := SpecFor(name)
	if !ok || s.Card == nil {
		return ""
	}
	return s.Card()
}

// Nav returns every service with a page, ordered by label. This is the
// catalogue at /services; the sidebar shows Pinned.
func Nav() []Spec {
	out := make([]Spec, 0, len(specs))
	for _, s := range Specs() {
		if s.Headless() {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NavLabel() < out[j].NavLabel() })
	return out
}

// Pinned returns the services named, in the order given, skipping any that are
// not registered or have no page to open.
//
// The order is the reader's and is preserved exactly — this is a list somebody
// arranged, not one to sort. A name that no longer resolves is dropped rather
// than rendered as a dead link, so removing a service from an instance quietly
// removes it from everyone's sidebar instead of breaking it.
//
// Two comments have described the sidebar as showing this since before it
// existed. It did not, which is why reaching for a service you use meant going
// to the catalogue and hunting for it.
func Pinned(names []string) []Spec {
	out := make([]Spec, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		s, ok := SpecFor(name)
		if !ok || s.Headless() {
			continue
		}
		out = append(out, s)
	}
	return out
}

var (
	specMu sync.RWMutex
	specs  = map[string]Spec{}
)

func recordSpec(s Spec) {
	specMu.Lock()
	specs[s.Name] = s
	specMu.Unlock()
}

// SpecFor returns a registered service's declaration.
func SpecFor(name string) (Spec, bool) {
	specMu.RLock()
	defer specMu.RUnlock()
	s, ok := specs[strings.ToLower(strings.TrimSpace(name))]
	return s, ok
}

// Specs returns every registered service's declaration, ordered by name.
func Specs() []Spec {
	specMu.RLock()
	defer specMu.RUnlock()
	out := make([]Spec, 0, len(specs))
	for _, s := range specs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AccountScoped reports whether a service requires an authenticated caller.
// Derived from the spec, so the agent and the app SDK cannot drift apart.
func AccountScoped(name string) bool {
	s, ok := SpecFor(name)
	return ok && s.Scoped
}

// Destructive reports whether a method is withheld from the model. Accepts
// "service.Method", "service_method" or a bare service and method.
func Destructive(service, method string) bool {
	s, ok := SpecFor(service)
	if !ok {
		return false
	}
	for name, ep := range s.Endpoints {
		if strings.EqualFold(name, method) {
			return ep.Destructive
		}
	}
	return false
}

// CostOf returns the wallet operation a method charges, or "" if it is free.
func CostOf(service, method string) string {
	s, ok := SpecFor(service)
	if !ok {
		return ""
	}
	for name, ep := range s.Endpoints {
		if strings.EqualFold(name, method) {
			return ep.Cost
		}
	}
	return ""
}

// Label returns a service's nav label.
func Label(name string) string {
	if s, ok := SpecFor(name); ok {
		return s.NavLabel()
	}
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// endpointOptions turns a Spec's endpoint declarations into the go-micro
// handler option that writes them into the registry, where tool discovery reads
// them.
//
// This exists because go-micro's own documentation cannot reach us. It fills
// endpoint metadata by locating a method's source file at runtime and parsing
// the Go doc comment (server/comments.go).
//
// That fails unconditionally here: handlers are registered as pointers
// (new(Server)) while their methods have value receivers, so reflection yields
// synthesised wrappers whose source file is "<autogenerated>" — the parse never
// reaches a real file.
//
// Fixing the receiver would not be enough either, because the mechanism needs
// the Go source tree to sit next to the binary at runtime. That happens to hold
// for the systemd deployment, which builds in place, and does not hold for the
// container image, which is a multi-stage build carrying only the binary. A
// tool description should not depend on how the binary was shipped.
//
// Without this, every tool the agent sees is described as "Call Search on web
// service", which is close to no description at all.
func endpointOptions(s Spec) server.HandlerOption {
	prefix := handlerName(s.Handler)
	if prefix == "" || len(s.Endpoints) == 0 {
		return nil
	}
	docs := map[string]server.EndpointDoc{}
	for method, ep := range s.Endpoints {
		if method == "" || ep.Doc == "" {
			continue
		}
		docs[prefix+"."+method] = server.EndpointDoc{Description: ep.Doc}
	}
	if len(docs) == 0 {
		return nil
	}
	return server.WithEndpointDocs(docs)
}

// HandlerName is the type name go-micro prefixes a handler's endpoints with —
// "Server" for both Server and *Server. Exported because deriving a tool from
// an endpoint needs the same "Server.Method" string the registry uses.
func HandlerName(h any) string { return handlerName(h) }

// handlerName is the type name go-micro prefixes a handler's endpoints with —
// "Server" for both Server and *Server.
func handlerName(h any) string {
	t := reflect.TypeOf(h)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// EndpointDescriptions returns each registered endpoint of a service with the
// description it publishes, read back from the live registry.
func EndpointDescriptions(name string) map[string]string {
	ensure()
	svcs, err := reg.GetService(name)
	if err != nil || len(svcs) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, s := range svcs {
		for _, ep := range s.Endpoints {
			if ep == nil || ep.Name == "" {
				continue
			}
			out[ep.Name] = ep.Metadata["description"]
		}
	}
	return out
}

// GuestAllowedTool reports whether a caller with no account may use a tool,
// derived from the service behind it.
//
// A tool name is service_method (news_search) or the native service.Handler.Method
// form, so the first segment names the service. A scoped service is closed to
// guests entirely; anything else is public.
//
// This replaced two hand-written allowlists — one in agent, one in agent/micro
// — that had to be edited in step with each other and with accountScoped, and
// were not. Tools with no service behind them (quran, blog_read) are not
// covered here; their callers keep a short explicit list.
func GuestAllowedTool(name string) bool {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(name)), func(r rune) bool {
		return r == '.' || r == '_'
	})
	if len(parts) == 0 {
		return false
	}
	s, ok := SpecFor(parts[0])
	if !ok {
		return false
	}
	return !s.Scoped
}
