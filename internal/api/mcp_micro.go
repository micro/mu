package api

import (
	"context"
	"net/http"

	gwmcp "go-micro.dev/v6/gateway/mcp"
)

// mcpReqKey threads the originating HTTP request to the resolver so per-tool
// guards, wallet metering and authenticated execution see the real caller.
type mcpReqKey struct{}

// mcpResolver builds a go-micro MCP manual resolver from the registered tools.
// go-micro owns the MCP protocol/transport; mu keeps ownership of which tools
// exist and how they execute — the per-IP guard, wallet metering and
// authenticated dispatch ExecuteTool performs. No framework internals are
// exposed (no store/broker tools).
func mcpResolver() gwmcp.Resolver {
	res := gwmcp.NewManualResolver()
	st := mcpTools()
	for i := range st {
		t := st[i]
		props := map[string]interface{}{}
		var required []string
		for _, p := range t.Params {
			props[p.Name] = map[string]interface{}{"type": p.Type, "description": p.Description}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		schema := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}

		name := t.Name
		walletOp := t.WalletOp
		res.Add(gwmcp.Tool{Name: name, Description: t.Description, InputSchema: schema},
			func(ctx context.Context, args map[string]interface{}) (*gwmcp.CallResult, error) {
				r, _ := ctx.Value(mcpReqKey{}).(*http.Request)

				// Per-tool pre-check (e.g. signup rate limit per IP) — protocol error.
				if ToolGuard != nil && r != nil {
					if err := ToolGuard(r, name); err != nil {
						return nil, &gwmcp.RPCError{Code: -32000, Message: err.Error()}
					}
				}
				// Wallet metering for charged tools — protocol error -32000.
				if walletOp != "" && QuotaCheck != nil && r != nil {
					ok, cost, err := QuotaCheck(r, walletOp)
					if !ok {
						msg := "Insufficient credits"
						if err != nil {
							msg = err.Error()
						} else {
							msg = formatCredits(name, cost)
						}
						return nil, &gwmcp.RPCError{Code: -32000, Message: msg}
					}
				}
				// Tool execution — a tool-level error is an isError result, not
				// a protocol error.
				text, isErr, err := ExecuteTool(r, name, args)
				if err != nil {
					return &gwmcp.CallResult{Text: err.Error(), IsError: true}, nil
				}
				return &gwmcp.CallResult{Text: text, IsError: isErr}, nil
			})
	}
	// Retired names must keep resolving. The gateway dispatches against the
	// names it was handed, so aliases have to be mapped back here — registering
	// them as tools would list them, which defeats the point of retiring them.
	aliases := map[string]string{}
	for i := range st {
		for _, a := range st[i].Aliases {
			aliases[a] = st[i].Name
		}
	}
	if len(aliases) == 0 {
		return res
	}
	return aliasResolver{inner: res, aliases: aliases}
}

// aliasResolver keeps renamed tools callable by their old names without showing
// those names in the catalogue.
type aliasResolver struct {
	inner   gwmcp.Resolver
	aliases map[string]string // retired name -> current name
}

func (a aliasResolver) List(ctx context.Context) ([]gwmcp.Tool, error) {
	return a.inner.List(ctx)
}

func (a aliasResolver) Call(ctx context.Context, name string, args map[string]any) (*gwmcp.CallResult, error) {
	if canonical, ok := a.aliases[name]; ok {
		name = canonical
	}
	return a.inner.Call(ctx, name, args)
}

func formatCredits(name string, cost int) string {
	return "Insufficient credits: " + name + " requires " + itoa(cost) + " credits"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// serveMCP serves a JSON-RPC MCP request through go-micro's gateway/mcp handler.
func serveMCP(w http.ResponseWriter, r *http.Request) {
	handler := gwmcp.NewHandler(mcpResolver(),
		gwmcp.WithServerInfo("mu", "1.0.0"),
		gwmcp.WithProtocolVersion(MCPVersion))
	ctx := context.WithValue(r.Context(), mcpReqKey{}, r)
	handler.ServeHTTP(w, r.WithContext(ctx))
}
