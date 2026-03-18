// Package handler provides a unified service registry for Mu.
//
// A service registers once and gets: HTTP routing, MCP tool exposure,
// command dispatch (for the prompt/stream interface), and API documentation.
package handler

import (
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Service defines a building block's registration.
type Service struct {
	Name        string       // unique identifier (e.g. "weather")
	Description string       // short human description
	Route       string       // HTTP route (e.g. "/weather")
	Handler     http.HandlerFunc
	Auth        bool         // true = requires authentication
	Icon        string       // emoji or short icon for UI shortcuts
	Commands    []Command    // prompt/stream command handlers
	Tools       []Tool       // MCP tools this service exposes
}

// Command defines a prompt command that dispatches to a service.
type Command struct {
	Match  string                                                          // keyword to match (e.g. "weather")
	Hint   string                                                          // short usage hint (e.g. "weather <location>")
	Handle func(w http.ResponseWriter, r *http.Request, args string) string // returns HTML response
}

// Tool defines an MCP tool exposed by a service.
type Tool struct {
	Name        string
	Description string
	Method      string // HTTP method
	Path        string // HTTP path (defaults to service route)
	WalletOp    string
	Params      []ToolParam
	Handle      func(args map[string]any) (string, error) // optional direct handler
}

// ToolParam defines a parameter for an MCP tool.
type ToolParam struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

var (
	mu       sync.RWMutex
	services []Service
	cmdIndex map[string]int // command keyword → index in services
)

func init() {
	cmdIndex = make(map[string]int)
}

// Register adds a service to the registry.
func Register(s Service) {
	mu.Lock()
	defer mu.Unlock()

	idx := len(services)
	services = append(services, s)

	// Index commands for dispatch
	for _, cmd := range s.Commands {
		cmdIndex[strings.ToLower(cmd.Match)] = idx
	}
}

// Dispatch routes an input string to the matching service command.
// Returns the HTML response and true if a command matched, or ("", false) if not.
func Dispatch(w http.ResponseWriter, r *http.Request, input string) (string, bool) {
	parts := strings.SplitN(input, " ", 2)
	keyword := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	mu.RLock()
	idx, ok := cmdIndex[keyword]
	if !ok {
		mu.RUnlock()
		return "", false
	}
	svc := services[idx]
	mu.RUnlock()

	// Find the matching command
	for _, cmd := range svc.Commands {
		if strings.ToLower(cmd.Match) == keyword {
			return cmd.Handle(w, r, args), true
		}
	}
	return "", false
}

// Services returns all registered services, sorted by name.
func Services() []Service {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]Service, len(services))
	copy(result, services)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Shortcuts returns services that have commands with icons, suitable for UI chips.
func Shortcuts() []Shortcut {
	mu.RLock()
	defer mu.RUnlock()

	var result []Shortcut
	for _, s := range services {
		if len(s.Commands) > 0 && s.Icon != "" {
			hint := ""
			if len(s.Commands) > 0 {
				hint = s.Commands[0].Hint
			}
			result = append(result, Shortcut{
				Name:    s.Name,
				Icon:    s.Icon,
				Command: s.Commands[0].Match,
				Hint:    hint,
			})
		}
	}
	return result
}

// Shortcut is a UI-friendly representation of a quick command.
type Shortcut struct {
	Name    string
	Icon    string
	Command string
	Hint    string
}

// GetTools returns all registered MCP tools across all services.
func GetTools() []Tool {
	mu.RLock()
	defer mu.RUnlock()

	var result []Tool
	for _, s := range services {
		result = append(result, s.Tools...)
	}
	return result
}

// GetRoutes returns all registered HTTP routes and their handlers.
func GetRoutes() []RouteEntry {
	mu.RLock()
	defer mu.RUnlock()

	var result []RouteEntry
	for _, s := range services {
		if s.Route != "" && s.Handler != nil {
			result = append(result, RouteEntry{
				Route:   s.Route,
				Handler: s.Handler,
				Auth:    s.Auth,
			})
		}
	}
	return result
}

// RouteEntry pairs a route with its handler and auth requirement.
type RouteEntry struct {
	Route   string
	Handler http.HandlerFunc
	Auth    bool
}
