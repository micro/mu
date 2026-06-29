// Package mesh is mu's go-micro runtime core.
//
// It owns the shared go-micro infrastructure — registry, client, broker,
// store — and hosts mu's domain capabilities as in-process go-micro services.
// Every domain (news, markets, weather, …) registers a handler here; the HTTP
// layer and the agent reach those capabilities by calling the service through
// this package, so go-micro is the spine and HTTP is only a front.
//
// Services run in-process behind an in-memory registry: adopting go-micro does
// not force mu to physically distribute. The same handlers can later be split
// into separate processes by swapping the registry, with no handler changes.
package mesh

import (
	"context"
	"os"
	"strings"
	"sync"

	"go-micro.dev/v6/broker"
	"go-micro.dev/v6/client"
	gwmcp "go-micro.dev/v6/gateway/mcp"
	"go-micro.dev/v6/registry"
	"go-micro.dev/v6/selector"
	"go-micro.dev/v6/server"
	"go-micro.dev/v6/service"
	"go-micro.dev/v6/store"
)

func init() {
	// In-process services advertise loopback and are reached over loopback.
	// If an HTTP(S)_PROXY is configured, Go's transport would otherwise route
	// those loopback dials through the proxy, which hijacks them. Ensure
	// loopback always bypasses the proxy. Runs at package init, before any
	// request is made, so the proxy-env cache reads the updated value.
	bypassProxyForLoopback()
}

func bypassProxyForLoopback() {
	const loopback = "127.0.0.1,localhost,::1,0.0.0.0"
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		cur := os.Getenv(key)
		if cur == "" {
			os.Setenv(key, loopback)
			continue
		}
		if !strings.Contains(cur, "127.0.0.1") {
			os.Setenv(key, cur+","+loopback)
		}
	}
}

var (
	mu       sync.Mutex
	inited   bool
	reg      registry.Registry
	cl       client.Client
	br       broker.Broker
	st       store.Store
	services []service.Service
)

// Init builds the shared go-micro infrastructure. It is idempotent and safe to
// call from multiple Load() functions; the first call wins.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if inited {
		return
	}
	reg = registry.NewMemoryRegistry()
	br = broker.NewMemoryBroker()
	_ = br.Connect()
	cl = client.NewClient(
		client.Registry(reg),
		client.Selector(selector.NewSelector(selector.Registry(reg))),
		client.Broker(br),
	)
	st = store.NewMemoryStore()
	inited = true
}

func ensure() {
	if !inited {
		Init()
	}
}

// Registry returns the shared service registry.
func Registry() registry.Registry { ensure(); return reg }

// Client returns the shared RPC client.
func Client() client.Client { ensure(); return cl }

// Broker returns the shared message broker.
func Broker() broker.Broker { ensure(); return br }

// Store returns the shared key-value store.
func Store() store.Store { ensure(); return st }

// Register stands up an in-process go-micro service of the given name hosting
// the provided handler structs, and starts it. Each handler's exported methods
// of the form func(ctx, *Req, *Rsp) error become RPC endpoints — and, via the
// agent and gateways, AI tools.
//
// It returns once the service is registered and reachable.
func Register(name string, handlers ...any) error {
	ensure()
	svc := service.New(
		service.Name(name),
		service.Address("127.0.0.1:0"), // in-process: advertise loopback only
		service.Registry(reg),
		service.Client(cl),
		service.Broker(br),
	)
	for _, h := range handlers {
		if err := svc.Handle(h); err != nil {
			return err
		}
	}
	if err := svc.Start(); err != nil {
		return err
	}
	mu.Lock()
	services = append(services, svc)
	mu.Unlock()
	return nil
}

// HandlerOpts registers handlers with go-micro server options (e.g. endpoint
// metadata). Most callers want Register.
func HandlerOpts(name string, h any, opts ...server.HandlerOption) error {
	ensure()
	svc := service.New(service.Name(name), service.Registry(reg), service.Client(cl), service.Broker(br))
	if err := svc.Handle(h, opts...); err != nil {
		return err
	}
	if err := svc.Start(); err != nil {
		return err
	}
	mu.Lock()
	services = append(services, svc)
	mu.Unlock()
	return nil
}

// Call invokes a service endpoint with typed request/response values.
//
//	var rsp weather.ForecastResponse
//	mesh.Call(ctx, "weather", "Weather.Forecast", &weather.ForecastRequest{...}, &rsp)
func Call(ctx context.Context, svcName, endpoint string, req, rsp any) error {
	ensure()
	return cl.Call(ctx, cl.NewRequest(svcName, endpoint, req), rsp)
}

// StartMCPGateway runs go-micro's MCP gateway on addr (e.g. ":4100"),
// exposing every registered service's methods as MCP tools — schemas derived
// from the handler signatures and @example doc-comments. It blocks; run it in a
// goroutine. This is additive: it stands the framework's gateway up alongside
// mu's existing /mcp for comparison, on its own port.
func StartMCPGateway(addr string) error {
	ensure()
	return gwmcp.Serve(gwmcp.Options{
		Registry: reg,
		Client:   cl,
		Address:  addr,
	})
}

// Services returns the names of the registered in-process go-micro services.
func Services() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, 0, len(services))
	for _, s := range services {
		out = append(out, s.Name())
	}
	return out
}

// Stop shuts down all hosted services. Used on graceful shutdown.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	for _, s := range services {
		_ = s.Stop()
	}
	services = nil
}
