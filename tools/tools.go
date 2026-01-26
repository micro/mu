package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Tool represents a capability that can be invoked by agent, API, or HTTP
type Tool struct {
	Name        string                                                        `json:"name"`
	Description string                                                        `json:"description"`
	Category    string                                                        `json:"category"`
	Input       map[string]Param                                              `json:"input,omitempty"`
	Output      map[string]Param                                              `json:"output,omitempty"`
	Handler     func(ctx context.Context, params map[string]any) (any, error) `json:"-"`

	// HTTP routing (optional - for API/web access)
	Path   string `json:"path,omitempty"`   // e.g., "/markets/price"
	Method string `json:"method,omitempty"` // GET, POST, etc.
}

// Param describes an input parameter
type Param struct {
	Type        string   `json:"type"` // "string", "number", "bool", "array"
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"`
}

var (
	registry = make(map[string]*Tool)
	mu       sync.RWMutex
)

// Register adds a tool to the registry
func Register(tool Tool) {
	mu.Lock()
	defer mu.Unlock()
	registry[tool.Name] = &tool
}

// List returns all registered tools sorted by name
func List() []*Tool {
	mu.RLock()
	defer mu.RUnlock()

	tools := make([]*Tool, 0, len(registry))
	for _, t := range registry {
		tools = append(tools, t)
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	return tools
}

// Get returns a tool by name
func Get(name string) *Tool {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}

// Call invokes a tool by name with the given parameters
func Call(ctx context.Context, name string, params map[string]any) (any, error) {
	tool := Get(name)
	if tool == nil {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	if tool.Handler == nil {
		return nil, fmt.Errorf("tool has no handler: %s", name)
	}

	// Validate required parameters
	for paramName, param := range tool.Input {
		if param.Required {
			if _, ok := params[paramName]; !ok {
				return nil, fmt.Errorf("missing required parameter: %s", paramName)
			}
		}
	}

	return tool.Handler(ctx, params)
}

// ByCategory returns tools filtered by category
func ByCategory(category string) []*Tool {
	mu.RLock()
	defer mu.RUnlock()

	var tools []*Tool
	for _, t := range registry {
		if t.Category == category {
			tools = append(tools, t)
		}
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	return tools
}

// Categories returns all unique categories
func Categories() []string {
	mu.RLock()
	defer mu.RUnlock()

	cats := make(map[string]bool)
	for _, t := range registry {
		if t.Category != "" {
			cats[t.Category] = true
		}
	}

	result := make([]string, 0, len(cats))
	for c := range cats {
		result = append(result, c)
	}
	sort.Strings(result)
	return result
}

func init() {
	// Register the tools.list meta-tool
	Register(Tool{
		Name:        "tools.list",
		Description: "List all available tools and their descriptions",
		Category:    "system",
		Input:       map[string]Param{},
		Output: map[string]Param{
			"tools": {Type: "array", Description: "List of available tools"},
		},
		Handler: func(ctx context.Context, params map[string]any) (any, error) {
			tools := List()
			var result []map[string]string
			for _, t := range tools {
				if t.Name == "tools.list" {
					continue // skip self
				}
				result = append(result, map[string]string{
					"name":        t.Name,
					"description": t.Description,
					"category":    t.Category,
				})
			}
			return map[string]any{
				"tools": result,
				"count": len(result),
			}, nil
		},
	})
}

// Context key for user ID
type contextKey string

const userKey contextKey = "userID"

// WithUser returns a context with the user ID set
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

// UserFromContext extracts the user ID from context
func UserFromContext(ctx context.Context) string {
	if v := ctx.Value(userKey); v != nil {
		return v.(string)
	}
	return ""
}
